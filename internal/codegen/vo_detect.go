package codegen

import "go/types"

func isScannerValuer(t types.Type) bool {
	return hasMethod(t, "Scan") && hasMethod(t, "Value")
}

func isMultiColumnMapper(t types.Type) bool {
	return hasMethod(t, "DrelColumns") && hasMethod(t, "DrelValues") && hasMethod(t, "DrelScanMulti")
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
