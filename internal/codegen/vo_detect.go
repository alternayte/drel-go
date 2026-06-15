package codegen

import "go/types"

func isScannerValuer(t types.Type) bool {
	return hasMethod(t, "Scan") && hasMethod(t, "Value")
}

func isMultiColumnMapper(t types.Type) bool {
	return hasMethod(t, "DrelColumns") && hasMethod(t, "DrelValues") && hasMethod(t, "DrelScanMulti")
}

// hasDrelColumnTypes reports whether a multi-column VO exposes the optional
// DrelColumnTypes() []string hook for explicit sub-column SQL types.
func hasDrelColumnTypes(t types.Type) bool {
	return hasMethod(t, "DrelColumnTypes")
}

// defaultMultiColTypes returns one "text" entry per sub-column name. The
// optional DrelColumnTypes() hook is detected via hasDrelColumnTypes; W2-G1
// defaults every sub-column to text, leaving explicit per-column type overrides
// to the db-tag type= work (W2-G6/G9).
func defaultMultiColTypes(names []string) []string {
	out := make([]string, len(names))
	for i := range out {
		out[i] = "text"
	}
	return out
}

func hasMethod(t types.Type, name string) bool {
	mset := types.NewMethodSet(t)
	if mset.Lookup(nil, name) != nil {
		return true
	}
	ptr := types.NewPointer(t)
	ptrMset := types.NewMethodSet(ptr)
	return ptrMset.Lookup(nil, name) != nil
}

func localTypeName(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return t.String()
	}
	return named.Obj().Name()
}

func typePkgPath(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return ""
	}
	return pkg.Path()
}

func isPointerType(t types.Type) bool {
	_, ok := t.(*types.Pointer)
	return ok
}

func isPrimitiveType(goType string) bool {
	switch goType {
	case "string", "bool",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"byte", "rune":
		return true
	}
	return false
}

// isSliceType reports whether t is a slice (after unwrapping a pointer).
func isSliceType(t types.Type) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	_, ok := t.Underlying().(*types.Slice)
	return ok
}

// isMapType reports whether t is a map (after unwrapping a pointer).
func isMapType(t types.Type) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	_, ok := t.Underlying().(*types.Map)
	return ok
}

// isJSONContainer reports whether t is a slice, map, array, or plain struct that
// should be mapped as a JSON column. []byte is excluded (it maps to bytea/BLOB,
// handled as a primitive elsewhere if ever added).
func isJSONContainer(t types.Type) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Byte {
			return false // []byte is bytea/BLOB, not JSON
		}
		return true
	case *types.Map, *types.Array, *types.Struct:
		return true
	default:
		return false
	}
}
