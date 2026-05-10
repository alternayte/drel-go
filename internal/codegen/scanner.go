package codegen

import (
	"fmt"
	"go/constant"
	"go/types"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

func ScanPackages(patterns []string, dir ...string) ([]ModelInfo, error) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedDeps | packages.NeedImports,
	}
	if len(dir) > 0 && dir[0] != "" {
		cfg.Dir = dir[0]
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("codegen: load packages: %w", err)
	}

	var models []ModelInfo
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("codegen: package %s has errors: %v", pkg.PkgPath, pkg.Errors[0])
		}
		pkgModels, err := scanPackage(pkg)
		if err != nil {
			return nil, err
		}
		models = append(models, pkgModels...)
	}
	return models, nil
}

func scanPackage(pkg *packages.Package) ([]ModelInfo, error) {
	var models []ModelInfo
	scope := pkg.Types.Scope()

	// Determine the filesystem directory from the package's Go files.
	var pkgDir string
	if len(pkg.GoFiles) > 0 {
		pkgDir = filepath.Dir(pkg.GoFiles[0])
	}

	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		st, ok := tn.Type().Underlying().(*types.Struct)
		if !ok {
			continue
		}

		pkType, hasModel := findModelEmbed(st)
		if !hasModel {
			continue
		}

		mi := ModelInfo{
			Name:    tn.Name(),
			PkgPath: pkg.PkgPath,
			PkgName: pkg.Name,
			PKType:  pkType,
			Dir:     pkgDir,
		}

		mi.TableName = inferTableName(tn.Name())
		mi.HasSoftDelete, mi.HasVersioned, mi.HasAudit = detectEmbeds(st)
		mi.Fields = extractFields(st)

		models = append(models, mi)
	}
	return models, nil
}

func findModelEmbed(st *types.Struct) (pkType string, found bool) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Embedded() {
			continue
		}
		named, ok := f.Type().(*types.Named)
		if !ok {
			continue
		}
		obj := named.Obj()
		if obj.Name() != "Model" {
			continue
		}
		if obj.Pkg() == nil || !strings.HasSuffix(obj.Pkg().Path(), "drel") {
			continue
		}
		targs := named.TypeArgs()
		if targs == nil || targs.Len() != 1 {
			continue
		}
		return targs.At(0).String(), true
	}
	return "", false
}

func detectEmbeds(st *types.Struct) (softDelete, versioned, audit bool) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Embedded() {
			continue
		}
		named, ok := f.Type().(*types.Named)
		if !ok {
			continue
		}
		switch named.Obj().Name() {
		case "SoftDelete":
			softDelete = true
		case "Versioned":
			versioned = true
		case "Audit":
			audit = true
		}
	}
	return
}

func extractFields(st *types.Struct) []FieldInfo {
	var fields []FieldInfo
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Embedded() {
			continue
		}
		tag := st.Tag(i)
		dbCol := parseDBTag(tag)
		relTag := parseRelTag(tag)
		if dbCol == "" && relTag == "" {
			continue
		}

		fi := FieldInfo{
			Name:       f.Name(),
			GoType:     f.Type().String(),
			ColumnName: dbCol,
			IsExported: f.Exported(),
			RelTag:     relTag,
		}

		if relTag != "" {
			fi.Relation = parseRelTagStructured(relTag)
			fi.LocalGoType = localTypeName(f.Type())
			if fi.Relation != nil && fi.Relation.Type == "belongs_to" {
				fi.Relation.TargetModel = targetModelName(f.Type())
			}
		}

		if dbCol != "" {
			goTypeStr := f.Type().String()
			if isPrimitiveType(goTypeStr) {
				fi.LocalGoType = goTypeStr
			} else {
				fi.LocalGoType = localTypeName(f.Type())
				if isScannerValuer(f.Type()) {
					fi.IsVO = true
				}
				if isMultiColumnMapper(f.Type()) {
					fi.IsMultiColVO = true
					fi.MultiColPrefix = dbCol
				}
				if !isPrimitiveType(goTypeStr) && !fi.IsVO && !fi.IsMultiColVO {
					enumValues := findEnumValues(f.Type())
					if len(enumValues) > 0 {
						fi.IsEnum = true
						fi.EnumValues = enumValues
					}
				}
			}
		}

		fields = append(fields, fi)
	}
	return fields
}

func parseRelTagStructured(tag string) *RelationFieldInfo {
	if tag == "" {
		return nil
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return nil
	}
	ri := &RelationFieldInfo{Type: parts[0]}
	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "fk":
			ri.FK = kv[1]
		case "join":
			ri.JoinTable = kv[1]
		}
	}
	return ri
}

func parseDBTag(rawTag string) string {
	st := reflect.StructTag(rawTag)
	return st.Get("db")
}

func parseRelTag(rawTag string) string {
	st := reflect.StructTag(rawTag)
	return st.Get("rel")
}

func findEnumValues(t types.Type) []string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	basic, ok := named.Underlying().(*types.Basic)
	if !ok {
		return nil
	}
	if basic.Kind() != types.String && !isIntegerBasicKind(basic.Kind()) {
		return nil
	}

	typePkg := named.Obj().Pkg()
	if typePkg == nil {
		return nil
	}

	scope := typePkg.Scope()
	var values []string
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		c, ok := obj.(*types.Const)
		if !ok {
			continue
		}
		if !types.Identical(c.Type(), named) {
			continue
		}
		val := c.Val().ExactString()
		if basic.Kind() == types.String {
			val = constant.StringVal(c.Val())
		}
		values = append(values, val)
	}
	sort.Strings(values)
	return values
}

func isIntegerBasicKind(k types.BasicKind) bool {
	switch k {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return true
	}
	return false
}

func targetModelName(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}
