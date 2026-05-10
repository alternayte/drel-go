package codegen

type ModelInfo struct {
	Name          string
	PkgPath       string
	PkgName       string
	PKType        string
	TableName     string
	Fields        []FieldInfo
	HasSoftDelete bool
	HasVersioned  bool
	HasAudit      bool
}

type FieldInfo struct {
	Name       string
	GoType     string
	ColumnName string
	IsExported bool
	RelTag     string
}
