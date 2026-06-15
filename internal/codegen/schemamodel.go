package codegen

import (
	"fmt"
	"sort"
	"strings"
)

// Schema is a JSON-serializable, dialect-resolved logical representation of the
// full database schema. It is the canonical input to the structured migration
// diff engine and the persisted migration snapshot.
type Schema struct {
	Tables []Table   `json:"tables"`
	Enums  []EnumDef `json:"enums,omitempty"`
}

// Table describes a single relation (table or pivot) and its columns/indexes.
// PrimaryKey, when non-empty, declares a composite (table-level) primary key
// used by many-to-many pivot tables; for regular tables the primary key is the
// single column whose PK field is true.
type Table struct {
	Name       string   `json:"name"`
	Columns    []Column `json:"columns"`
	Indexes    []Index  `json:"indexes,omitempty"`
	PrimaryKey []string `json:"primaryKey,omitempty"`
}

// Column describes a single column. Type is the dialect-resolved SQL type
// (already including any quoted enum type name). Ref, when non-empty, is the
// referenced table name for a foreign key on this column (always referencing
// that table's "id").
type Column struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"notNull"`
	Default string `json:"default,omitempty"`
	Check   string `json:"check,omitempty"`
	Ref     string `json:"ref,omitempty"`
	PK      bool   `json:"pk,omitempty"`
}

// Index describes a single (possibly composite, possibly unique) index.
type Index struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
}

// EnumDef describes a Postgres enum type. Name is the lower-cased local Go type
// name; it is emitted as a CREATE TYPE statement (Postgres only).
type EnumDef struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// BuildSchema produces the full logical schema for the given models and dialect.
// It includes all tables (models + many-to-many pivots), all columns (user
// columns + trait columns), all indexes (explicit/unique/composite from db tag
// options), and enum type definitions (Postgres only).
//
// Ordering is deterministic: tables preserve model declaration order (pivots
// appended after, in discovery order), columns preserve PK-first then field
// order then trait order, and indexes within a table are sorted by name.
func BuildSchema(models []ModelInfo, dialect string) Schema {
	fks := collectFKs(models)

	var s Schema

	if dialect != "sqlite" {
		s.Enums = buildEnums(models)
	}

	for _, m := range models {
		s.Tables = append(s.Tables, buildTable(m, fks, dialect))
	}
	for _, t := range buildPivotTables(models, dialect) {
		s.Tables = append(s.Tables, t)
	}

	return s
}

// buildEnums collects enum type definitions across all models, de-duplicated by
// lower-cased local type name, in first-seen order.
func buildEnums(models []ModelInfo) []EnumDef {
	seen := map[string]bool{}
	var enums []EnumDef
	for _, m := range models {
		for _, f := range columnFields(m.Fields) {
			if f.IsEnum && len(f.EnumValues) > 0 {
				name := strings.ToLower(f.LocalGoType)
				if seen[name] {
					continue
				}
				seen[name] = true
				vals := append([]string(nil), f.EnumValues...)
				enums = append(enums, EnumDef{Name: name, Values: vals})
			}
		}
	}
	return enums
}

// buildTable builds the structured Table for a single model, including PK,
// user columns, trait columns, and indexes derived from db tag options.
func buildTable(m ModelInfo, fks map[string]string, dialect string) Table {
	t := Table{Name: m.TableName}

	// Primary key column.
	pk := Column{Name: "id", PK: true, NotNull: true}
	if dialect == "sqlite" {
		switch m.PKType {
		case "int", "int8", "int16", "int32", "int64":
			pk.Type = "INTEGER PRIMARY KEY AUTOINCREMENT"
		default:
			pk.Type = GoTypeToSQL(m.PKType, dialect) + " PRIMARY KEY"
		}
	} else {
		switch m.PKType {
		case "int", "int32":
			pk.Type = "SERIAL PRIMARY KEY"
		case "int64":
			pk.Type = "BIGSERIAL PRIMARY KEY"
		default:
			pk.Type = GoTypeToSQL(m.PKType, dialect) + " PRIMARY KEY"
		}
	}
	t.Columns = append(t.Columns, pk)

	// User-defined columns.
	for _, f := range columnFields(m.Fields) {
		if f.IsMultiColVO {
			notNull := !strings.HasPrefix(f.GoType, "*")
			for i, sub := range f.MultiColNames {
				sqlType := "text"
				if dialect == "sqlite" {
					sqlType = "TEXT"
				}
				if i < len(f.MultiColTypes) && f.MultiColTypes[i] != "" {
					sqlType = GoTypeToSQL(f.MultiColTypes[i], dialect)
				}
				sc := Column{Name: sub, Type: sqlType, NotNull: notNull}
				if fks != nil {
					if target, ok := fks[sub]; ok {
						sc.Ref = target
					}
				}
				t.Columns = append(t.Columns, sc)
			}
			continue
		}

		c := Column{Name: f.ColumnName, NotNull: !strings.HasPrefix(f.GoType, "*")}
		sqlType := GoTypeToSQL(f.GoType, dialect)
		if f.IsVO && f.VOBaseType != "" {
			// Single-column VOs store the VO's underlying primitive, not a struct;
			// resolve the column type from that primitive instead of defaulting to text.
			sqlType = GoTypeToSQL(f.VOBaseType, dialect)
		}
		if f.IsEnum {
			if dialect == "sqlite" {
				sqlType = "TEXT"
			} else {
				sqlType = quoteIdent(strings.ToLower(f.LocalGoType))
			}
		}
		c.Type = sqlType

		if f.IsEnum && dialect == "sqlite" && len(f.EnumValues) > 0 {
			c.Check = fmt.Sprintf("%s IN (%s)", quoteIdent(f.ColumnName), quoteEnumValues(f.EnumValues))
		} else if f.CheckExpr != "" {
			c.Check = f.CheckExpr
		}

		if fks != nil {
			if target, ok := fks[f.ColumnName]; ok {
				c.Ref = target
			}
		}

		t.Columns = append(t.Columns, c)
	}

	// Trait columns.
	if dialect == "sqlite" {
		if m.HasSoftDelete {
			t.Columns = append(t.Columns, Column{Name: "deleted_at", Type: "DATETIME"})
		}
		if m.HasVersioned {
			t.Columns = append(t.Columns, Column{Name: "version", Type: "INTEGER", NotNull: true, Default: "1"})
		}
		if m.HasAudit {
			t.Columns = append(t.Columns, Column{Name: "created_by", Type: "TEXT"})
			t.Columns = append(t.Columns, Column{Name: "updated_by", Type: "TEXT"})
		}
		t.Columns = append(t.Columns, Column{Name: "created_at", Type: "DATETIME", NotNull: true, Default: "CURRENT_TIMESTAMP"})
		t.Columns = append(t.Columns, Column{Name: "updated_at", Type: "DATETIME", NotNull: true, Default: "CURRENT_TIMESTAMP"})
	} else {
		if m.HasSoftDelete {
			t.Columns = append(t.Columns, Column{Name: "deleted_at", Type: "timestamptz"})
		}
		if m.HasVersioned {
			t.Columns = append(t.Columns, Column{Name: "version", Type: "integer", NotNull: true, Default: "1"})
		}
		if m.HasAudit {
			t.Columns = append(t.Columns, Column{Name: "created_by", Type: "text"})
			t.Columns = append(t.Columns, Column{Name: "updated_by", Type: "text"})
		}
		t.Columns = append(t.Columns, Column{Name: "created_at", Type: "timestamptz", NotNull: true, Default: "NOW()"})
		t.Columns = append(t.Columns, Column{Name: "updated_at", Type: "timestamptz", NotNull: true, Default: "NOW()"})
	}

	t.Indexes = buildIndexes(m)
	return t
}

// buildIndexes derives indexes from db tag options on a model's columns.
//
//   - Fields sharing an explicit IndexName compose ONE index, columns ordered by
//     field declaration order. The composite is unique if ANY member is marked
//     unique.
//   - A field marked Indexed without an explicit name gets a single-column index
//     auto-named idx_<table>_<col>.
//   - A field marked unique (without an explicit index name) gets a unique
//     single-column index uq_<table>_<col>. A unique field that also belongs to a
//     named index does not get a separate uq_ index (the named index carries the
//     uniqueness).
//
// Indexes are returned sorted by name for determinism.
func buildIndexes(m ModelInfo) []Index {
	type group struct {
		columns []string
		unique  bool
	}
	named := map[string]*group{}
	var namedOrder []string
	var indexes []Index

	for _, f := range columnFields(m.Fields) {
		if f.IndexName != "" {
			g, ok := named[f.IndexName]
			if !ok {
				g = &group{}
				named[f.IndexName] = g
				namedOrder = append(namedOrder, f.IndexName)
			}
			g.columns = append(g.columns, f.ColumnName)
			if f.Unique {
				g.unique = true
			}
			continue
		}
		if f.Indexed {
			indexes = append(indexes, Index{
				Name:    fmt.Sprintf("idx_%s_%s", m.TableName, f.ColumnName),
				Columns: []string{f.ColumnName},
			})
		}
		if f.Unique {
			indexes = append(indexes, Index{
				Name:    fmt.Sprintf("uq_%s_%s", m.TableName, f.ColumnName),
				Columns: []string{f.ColumnName},
				Unique:  true,
			})
		}
	}

	for _, name := range namedOrder {
		g := named[name]
		indexes = append(indexes, Index{Name: name, Columns: g.columns, Unique: g.unique})
	}

	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
	return indexes
}

// buildPivotTables builds structured Tables for many-to-many join tables,
// de-duplicated by join-table name, in discovery order.
func buildPivotTables(models []ModelInfo, dialect string) []Table {
	modelTable := map[string]string{}
	modelPK := map[string]string{}
	for _, m := range models {
		modelTable[m.Name] = m.TableName
		modelPK[m.Name] = m.PKType
	}

	seen := map[string]bool{}
	var pivots []Table

	for _, m := range models {
		for _, f := range m.Fields {
			if f.Relation == nil || f.Relation.Type != "many_to_many" {
				continue
			}
			jt := f.Relation.JoinTable
			if jt == "" || seen[jt] {
				continue
			}
			targetTable, ok := modelTable[f.Relation.TargetModel]
			if !ok {
				continue
			}
			seen[jt] = true

			srcPKType := GoTypeToSQL(m.PKType, dialect)
			tgtPKType := GoTypeToSQL(modelPK[f.Relation.TargetModel], dialect)

			pivots = append(pivots, Table{
				Name: jt,
				Columns: []Column{
					{Name: f.Relation.FK, Type: srcPKType, NotNull: true, Ref: m.TableName},
					{Name: f.Relation.RefColumn, Type: tgtPKType, NotNull: true, Ref: targetTable},
				},
				PrimaryKey: []string{f.Relation.FK, f.Relation.RefColumn},
			})
		}
	}
	return pivots
}

// quoteEnumValues renders enum values as a SQL-quoted comma-separated list.
func quoteEnumValues(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	}
	return strings.Join(quoted, ", ")
}
