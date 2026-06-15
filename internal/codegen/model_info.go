package codegen

type ModelInfo struct {
	Name          string
	PkgPath       string
	PkgName       string
	PKType        string // display type for generated code (e.g., "int", "uuid.UUID")
	PKTypeFull    string // fully qualified (e.g., "github.com/google/uuid.UUID")
	PKTypePkg     string // import path for external PK types (empty for primitives)
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
	MultiColNames  []string // expanded sub-column names from the db tag (multi-column VOs)
	MultiColTypes  []string // resolved SQL types per sub-column (default "text"; multi-column VOs)
	LocalGoType    string // type name without package qualifier, for same-package generated code
	TypePkgPath    string // import path of the field type's package (empty for primitives/same-package)
	IsPointer      bool   // whether the field is a pointer type
	IsEnum         bool
	EnumValues     []string
	Unique         bool   // db tag option: unique index on this column
	Indexed        bool   // db tag option: index on this column
	IndexName      string // explicit index name (db:"...,index=name"); fields sharing a name form a composite index
	CheckExpr      string // db tag option: column CHECK constraint expression (db:"...,check=expr")
}

// RelationFieldInfo holds parsed relationship metadata from a `rel:"..."` struct tag.
type RelationFieldInfo struct {
	Type        string // "has_many", "has_one", "belongs_to", "many_to_many"
	FK          string // foreign key column
	JoinTable   string // for many_to_many
	RefColumn   string // target FK column in pivot table (many-to-many)
	TargetModel string // target model name for foreign key resolution
}
