package codegen

import (
	"fmt"
	"go/constant"
	"go/types"
	"path"
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

		pkInfo, hasModel := findModelEmbed(st)
		if !hasModel {
			continue
		}

		mi := ModelInfo{
			Name:       tn.Name(),
			PkgPath:    pkg.PkgPath,
			PkgName:    pkg.Name,
			PKType:     pkInfo.Display,
			PKTypeFull: pkInfo.Full,
			PKTypePkg:  pkInfo.PkgPath,
			Dir:        pkgDir,
		}

		switch mi.PKType {
		case "uint", "uint8", "uint16", "uint32", "uint64":
			return nil, fmt.Errorf(
				"codegen: model %s: unsigned integer primary keys (%s) are not supported; use a signed integer (auto-increment) or uuid.UUID / string (app-assigned)",
				mi.Name, mi.PKType)
		}

		mi.TableName = inferTableName(tn.Name())
		mi.HasSoftDelete, mi.HasVersioned, mi.HasAudit = detectEmbeds(st)
		mi.Fields = extractFields(st, pkg.PkgPath)

		// Populate m2m convention defaults for relationship fields.
		for j := range mi.Fields {
			f := &mi.Fields[j]
			if f.Relation != nil && f.Relation.Type == "many_to_many" {
				sourceTable := inferTableName(tn.Name())
				targetTable := inferTableName(f.Relation.TargetModel)
				if f.Relation.JoinTable == "" {
					a, b := sourceTable, targetTable
					if a > b {
						a, b = b, a
					}
					f.Relation.JoinTable = singularize(a) + "_" + singularize(b)
				}
				if f.Relation.FK == "" {
					f.Relation.FK = singularize(sourceTable) + "_id"
				}
				if f.Relation.RefColumn == "" {
					f.Relation.RefColumn = singularize(targetTable) + "_id"
				}
			}
		}

		models = append(models, mi)
	}
	return models, nil
}

type pkTypeInfo struct {
	Display string // short name for generated code (e.g., "int", "uuid.UUID")
	Full    string // fully qualified (e.g., "github.com/google/uuid.UUID")
	PkgPath string // import path (empty for primitives)
}

func findModelEmbed(st *types.Struct) (info pkTypeInfo, found bool) {
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
		t := targs.At(0)
		full := t.String()
		display := full
		pkgPath := ""
		if n, ok := t.(*types.Named); ok {
			display = localTypeName(t)
			if p := n.Obj().Pkg(); p != nil {
				pkgPath = p.Path()
				if pkgPath != "" {
					display = path.Base(pkgPath) + "." + display
				}
			}
		}
		return pkTypeInfo{Display: display, Full: full, PkgPath: pkgPath}, true
	}
	return pkTypeInfo{}, false
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

func extractFields(st *types.Struct, ownerPkgPath string) []FieldInfo {
	var fields []FieldInfo
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Embedded() {
			continue
		}
		tag := st.Tag(i)
		dbCol, dbOpts := parseDBTag(tag)
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
			Unique:     dbOpts.unique,
			Indexed:    dbOpts.indexed,
			IndexName:  dbOpts.indexName,
			CheckExpr:  dbOpts.check,
		}

		if relTag != "" {
			fi.Relation = parseRelTagStructured(relTag)
			fi.LocalGoType = localTypeName(f.Type())
			if fi.Relation != nil {
				fi.Relation.TargetModel = targetModelName(f.Type())
			}
		}

		if dbCol != "" {
			goTypeStr := f.Type().String()
			if isPrimitiveType(goTypeStr) {
				fi.LocalGoType = goTypeStr
			} else {
				fi.LocalGoType = localTypeName(f.Type())
				fieldPkg := typePkgPath(f.Type())
				if fieldPkg != ownerPkgPath {
					fi.TypePkgPath = fieldPkg
				}
				fi.IsPointer = isPointerType(f.Type())
				if isScannerValuer(f.Type()) {
					fi.IsVO = true
				}
				if isMultiColumnMapper(f.Type()) {
					fi.IsMultiColVO = true
					names := splitMultiColNames(rawDBTag(tag))
					fi.MultiColNames = names
					fi.MultiColPrefix = dbCol
					if len(names) > 0 {
						// Keep ColumnName as the first sub-column so columnFields()
						// continues to include this field; the full set lives in
						// MultiColNames. Options (unique/index/check) are NOT parsed
						// for multi-col VOs — every segment is a column name.
						fi.ColumnName = names[0]
						fi.Unique = false
						fi.Indexed = false
						fi.IndexName = ""
						fi.CheckExpr = ""
					}
					if hasDrelColumnTypes(f.Type()) {
						fi.MultiColTypes = defaultMultiColTypes(names)
					} else {
						fi.MultiColTypes = defaultMultiColTypes(names)
					}
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
		case "ref":
			ri.RefColumn = kv[1]
		}
	}
	return ri
}

// dbTagOpts holds the options parsed from a db struct tag after the column name.
type dbTagOpts struct {
	unique    bool
	indexed   bool
	indexName string
	check     string
}

// parseDBTag splits a db struct tag into its column name and options. The first
// comma-separated element is the column name; subsequent elements are options:
//
//	db:"email,unique"                 — unique index
//	db:"age,index"                    — single-column index (auto-named)
//	db:"role,index=idx_role_age"      — named index; fields sharing the name compose
//	db:"age,check=age >= 0"           — column CHECK constraint
//
// Returns an empty column name when no db tag is present.
func parseDBTag(rawTag string) (string, dbTagOpts) {
	st := reflect.StructTag(rawTag)
	raw, ok := st.Lookup("db")
	if !ok || raw == "" {
		return "", dbTagOpts{}
	}
	parts := strings.Split(raw, ",")
	col := strings.TrimSpace(parts[0])
	var opts dbTagOpts
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		switch {
		case p == "unique":
			opts.unique = true
		case p == "index":
			opts.indexed = true
		case strings.HasPrefix(p, "index="):
			opts.indexed = true
			opts.indexName = strings.TrimSpace(strings.TrimPrefix(p, "index="))
		case strings.HasPrefix(p, "check="):
			opts.check = strings.TrimSpace(strings.TrimPrefix(p, "check="))
		}
	}
	return col, opts
}

// rawDBTag returns the raw db struct tag value (the full comma list, unsplit).
func rawDBTag(rawTag string) string {
	st := reflect.StructTag(rawTag)
	v, ok := st.Lookup("db")
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// splitMultiColNames splits a multi-column VO db tag into its sub-column names,
// trimming whitespace and dropping empty segments.
func splitMultiColNames(rawDB string) []string {
	parts := strings.Split(rawDB, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
	// Unwrap slice for has_many relations (e.g. []Post or []*Post).
	if sl, ok := t.(*types.Slice); ok {
		t = sl.Elem()
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}
