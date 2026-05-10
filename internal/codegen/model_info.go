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
	Dir           string // filesystem directory of the package
}

type FieldInfo struct {
	Name           string
	GoType         string
	ColumnName     string
	IsExported     bool
	RelTag         string
	Relation       *RelationFieldInfo
	IsVO           bool   // implements sql.Scanner + driver.Valuer (single-column VO)
	IsMultiColVO   bool   // implements drel.MultiColumnMapper (multi-column VO)
	MultiColPrefix string // db tag used as column prefix for multi-column VOs
	LocalGoType    string // type name without package qualifier, for same-package generated code
}

// RelationFieldInfo holds parsed relationship metadata from a `rel:"..."` struct tag.
type RelationFieldInfo struct {
	Type      string // "has_many", "has_one", "belongs_to", "many_to_many"
	FK        string // foreign key column
	JoinTable string // for many_to_many
}
