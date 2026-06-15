package drel

// EventTypeNameForTest exposes eventTypeName to the external test package.
func EventTypeNameForTest(v any) (string, error) { return eventTypeName(v) }
