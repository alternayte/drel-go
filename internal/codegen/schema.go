package codegen

import (
	"fmt"
	"strings"
)

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// GoTypeToSQL maps a Go type string to its corresponding Postgres SQL type.
// Pointer types are unwrapped to their base type. Unknown types default to "text".
func GoTypeToSQL(goType string) string {
	if strings.HasPrefix(goType, "*") {
		return GoTypeToSQL(goType[1:])
	}
	switch goType {
	case "int", "int32":
		return "integer"
	case "int8", "int16":
		return "smallint"
	case "int64":
		return "bigint"
	case "string":
		return "text"
	case "bool":
		return "boolean"
	case "float32":
		return "real"
	case "float64":
		return "double precision"
	case "time.Time":
		return "timestamptz"
	case "github.com/google/uuid.UUID", "uuid.UUID":
		return "uuid"
	default:
		return "text"
	}
}

// GenerateCreateTable emits a CREATE TABLE statement for a single model.
// fks maps column names to referenced table names for foreign key constraints.
func GenerateCreateTable(m ModelInfo, fks map[string]string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", quoteIdent(m.TableName)))

	switch m.PKType {
	case "int", "int32":
		b.WriteString(fmt.Sprintf("    %s SERIAL PRIMARY KEY", quoteIdent("id")))
	case "int64":
		b.WriteString(fmt.Sprintf("    %s BIGSERIAL PRIMARY KEY", quoteIdent("id")))
	default:
		b.WriteString(fmt.Sprintf("    %s %s PRIMARY KEY", quoteIdent("id"), GoTypeToSQL(m.PKType)))
	}

	for _, f := range columnFields(m.Fields) {
		nullable := strings.HasPrefix(f.GoType, "*")
		sqlType := GoTypeToSQL(f.GoType)
		if f.IsEnum {
			sqlType = quoteIdent(strings.ToLower(f.LocalGoType))
		}
		ref := ""
		if fks != nil {
			if target, ok := fks[f.ColumnName]; ok {
				ref = fmt.Sprintf(" REFERENCES %s(%s)", quoteIdent(target), quoteIdent("id"))
			}
		}
		if nullable {
			b.WriteString(fmt.Sprintf(",\n    %s %s%s", quoteIdent(f.ColumnName), sqlType, ref))
		} else {
			b.WriteString(fmt.Sprintf(",\n    %s %s NOT NULL%s", quoteIdent(f.ColumnName), sqlType, ref))
		}
	}

	if m.HasSoftDelete {
		b.WriteString(fmt.Sprintf(",\n    %s timestamptz", quoteIdent("deleted_at")))
	}
	if m.HasVersioned {
		b.WriteString(fmt.Sprintf(",\n    %s integer NOT NULL DEFAULT 1", quoteIdent("version")))
	}
	if m.HasAudit {
		b.WriteString(fmt.Sprintf(",\n    %s text", quoteIdent("created_by")))
		b.WriteString(fmt.Sprintf(",\n    %s text", quoteIdent("updated_by")))
	}

	b.WriteString(fmt.Sprintf(",\n    %s timestamptz NOT NULL DEFAULT NOW()", quoteIdent("created_at")))
	b.WriteString(fmt.Sprintf(",\n    %s timestamptz NOT NULL DEFAULT NOW()", quoteIdent("updated_at")))
	b.WriteString("\n);\n")

	return b.String()
}

// GenerateSchema emits the full schema DDL for a slice of models, including
// enum type definitions, foreign key constraints, and many-to-many pivot tables.
func GenerateSchema(models []ModelInfo) string {
	fks := collectFKs(models)
	enums := collectEnums(models)
	pivots := collectPivotTables(models)

	var b strings.Builder
	for _, e := range enums {
		b.WriteString(e)
		b.WriteString("\n")
	}
	for i, m := range models {
		if i > 0 || len(enums) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(GenerateCreateTable(m, fks))
	}
	for _, p := range pivots {
		b.WriteString("\n")
		b.WriteString(p)
	}
	return b.String()
}

// collectPivotTables generates CREATE TABLE DDL for many-to-many join tables.
// Each join table is emitted only once, even when both sides of the relation
// reference the same JoinTable name.
func collectPivotTables(models []ModelInfo) []string {
	modelTable := map[string]string{}
	modelPK := map[string]string{}
	for _, m := range models {
		modelTable[m.Name] = m.TableName
		modelPK[m.Name] = m.PKType
	}

	seen := map[string]bool{}
	var pivots []string

	for _, m := range models {
		for _, f := range m.Fields {
			if f.Relation == nil || f.Relation.Type != "many_to_many" {
				continue
			}
			jt := f.Relation.JoinTable
			if jt == "" || seen[jt] {
				continue
			}
			seen[jt] = true

			targetTable, ok := modelTable[f.Relation.TargetModel]
			if !ok {
				continue
			}

			srcPKType := GoTypeToSQL(m.PKType)
			tgtPKType := GoTypeToSQL(modelPK[f.Relation.TargetModel])

			var b strings.Builder
			b.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", quoteIdent(jt)))
			b.WriteString(fmt.Sprintf("    %s %s NOT NULL REFERENCES %s(%s),\n",
				quoteIdent(f.Relation.FK), srcPKType, quoteIdent(m.TableName), quoteIdent("id")))
			b.WriteString(fmt.Sprintf("    %s %s NOT NULL REFERENCES %s(%s),\n",
				quoteIdent(f.Relation.RefColumn), tgtPKType, quoteIdent(targetTable), quoteIdent("id")))
			b.WriteString(fmt.Sprintf("    PRIMARY KEY (%s, %s)\n",
				quoteIdent(f.Relation.FK), quoteIdent(f.Relation.RefColumn)))
			b.WriteString(");\n")

			pivots = append(pivots, b.String())
		}
	}
	return pivots
}

// GenerateDropSchema emits DROP TABLE statements in reverse order to respect
// foreign key dependencies.
func GenerateDropSchema(models []ModelInfo) string {
	var b strings.Builder
	for i := len(models) - 1; i >= 0; i-- {
		b.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", quoteIdent(models[i].TableName)))
	}
	return b.String()
}

// collectFKs builds a map of column name → referenced table for belongs_to relations.
func collectFKs(models []ModelInfo) map[string]string {
	modelTable := map[string]string{}
	for _, m := range models {
		modelTable[m.Name] = m.TableName
	}

	fks := map[string]string{}
	for _, m := range models {
		for _, f := range m.Fields {
			if f.Relation != nil && f.Relation.Type == "belongs_to" && f.Relation.FK != "" && f.Relation.TargetModel != "" {
				if target, ok := modelTable[f.Relation.TargetModel]; ok {
					fks[f.Relation.FK] = target
				}
			}
		}
	}
	return fks
}

// collectEnums discovers enum fields across all models and returns CREATE TYPE statements.
func collectEnums(models []ModelInfo) []string {
	seen := map[string]bool{}
	var enums []string
	for _, m := range models {
		for _, f := range columnFields(m.Fields) {
			if f.IsEnum && len(f.EnumValues) > 0 {
				name := strings.ToLower(f.LocalGoType)
				if seen[name] {
					continue
				}
				seen[name] = true
				quoted := make([]string, len(f.EnumValues))
				for i, v := range f.EnumValues {
					quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
				}
				enums = append(enums, fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", quoteIdent(name), strings.Join(quoted, ", ")))
			}
		}
	}
	return enums
}
