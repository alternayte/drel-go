package codegen

import (
	"fmt"
	"strings"
)

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// GoTypeToSQL maps a Go type string to its corresponding SQL type for the given dialect.
// Pointer types are unwrapped to their base type. Unknown types default to "text"/"TEXT".
// Supported dialects: "postgres" (default), "sqlite".
func GoTypeToSQL(goType string, dialect string) string {
	if strings.HasPrefix(goType, "*") {
		return GoTypeToSQL(goType[1:], dialect)
	}
	if dialect == "sqlite" {
		return goTypeToSQLite(goType)
	}
	return goTypeToPostgres(goType)
}

func goTypeToPostgres(goType string) string {
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

func goTypeToSQLite(goType string) string {
	switch goType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return "INTEGER"
	case "string":
		return "TEXT"
	case "bool":
		return "INTEGER"
	case "float32", "float64":
		return "REAL"
	case "time.Time":
		return "DATETIME"
	case "github.com/google/uuid.UUID", "uuid.UUID":
		return "TEXT"
	default:
		return "TEXT"
	}
}

// GenerateCreateTable emits a CREATE TABLE statement for a single model.
// fks maps column names to referenced table names for foreign key constraints.
// dialect controls SQL type mappings and syntax ("postgres" or "sqlite").
func GenerateCreateTable(m ModelInfo, fks map[string]string, dialect string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", quoteIdent(m.TableName)))

	if dialect == "sqlite" {
		switch m.PKType {
		case "int", "int8", "int16", "int32", "int64":
			b.WriteString(fmt.Sprintf("    %s INTEGER PRIMARY KEY AUTOINCREMENT", quoteIdent("id")))
		default:
			b.WriteString(fmt.Sprintf("    %s %s PRIMARY KEY", quoteIdent("id"), GoTypeToSQL(m.PKType, dialect)))
		}
	} else {
		switch m.PKType {
		case "int", "int32":
			b.WriteString(fmt.Sprintf("    %s SERIAL PRIMARY KEY", quoteIdent("id")))
		case "int64":
			b.WriteString(fmt.Sprintf("    %s BIGSERIAL PRIMARY KEY", quoteIdent("id")))
		default:
			b.WriteString(fmt.Sprintf("    %s %s PRIMARY KEY", quoteIdent("id"), GoTypeToSQL(m.PKType, dialect)))
		}
	}

	for _, f := range columnFields(m.Fields) {
		nullable := strings.HasPrefix(f.GoType, "*")
		sqlType := GoTypeToSQL(f.GoType, dialect)
		if f.IsEnum {
			if dialect == "sqlite" {
				// SQLite has no CREATE TYPE; use TEXT with a CHECK constraint.
				sqlType = "TEXT"
			} else {
				sqlType = quoteIdent(strings.ToLower(f.LocalGoType))
			}
		}
		ref := ""
		if fks != nil {
			if target, ok := fks[f.ColumnName]; ok {
				ref = fmt.Sprintf(" REFERENCES %s(%s)", quoteIdent(target), quoteIdent("id"))
			}
		}
		// For SQLite enums, append a CHECK constraint after the column definition.
		enumCheck := ""
		if f.IsEnum && dialect == "sqlite" && len(f.EnumValues) > 0 {
			quoted := make([]string, len(f.EnumValues))
			for i, v := range f.EnumValues {
				quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
			}
			enumCheck = fmt.Sprintf(" CHECK(%s IN (%s))", quoteIdent(f.ColumnName), strings.Join(quoted, ", "))
		}
		if nullable {
			b.WriteString(fmt.Sprintf(",\n    %s %s%s%s", quoteIdent(f.ColumnName), sqlType, enumCheck, ref))
		} else {
			b.WriteString(fmt.Sprintf(",\n    %s %s NOT NULL%s%s", quoteIdent(f.ColumnName), sqlType, enumCheck, ref))
		}
	}

	if dialect == "sqlite" {
		if m.HasSoftDelete {
			b.WriteString(fmt.Sprintf(",\n    %s DATETIME", quoteIdent("deleted_at")))
		}
		if m.HasVersioned {
			b.WriteString(fmt.Sprintf(",\n    %s INTEGER NOT NULL DEFAULT 1", quoteIdent("version")))
		}
		if m.HasAudit {
			b.WriteString(fmt.Sprintf(",\n    %s TEXT", quoteIdent("created_by")))
			b.WriteString(fmt.Sprintf(",\n    %s TEXT", quoteIdent("updated_by")))
		}
		b.WriteString(fmt.Sprintf(",\n    %s DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP", quoteIdent("created_at")))
		b.WriteString(fmt.Sprintf(",\n    %s DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP", quoteIdent("updated_at")))
	} else {
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
	}

	b.WriteString("\n);\n")

	return b.String()
}

// GenerateSchema emits the full schema DDL for a slice of models, including
// enum type definitions, foreign key constraints, and many-to-many pivot tables.
// dialect controls SQL type mappings and syntax ("postgres" or "sqlite").
func GenerateSchema(models []ModelInfo, dialect string) string {
	fks := collectFKs(models)
	pivots := collectPivotTables(models, dialect)

	var b strings.Builder

	// SQLite has no CREATE TYPE; enums are handled inline via CHECK constraints.
	var enums []string
	if dialect != "sqlite" {
		enums = collectEnums(models)
		for _, e := range enums {
			b.WriteString(e)
			b.WriteString("\n")
		}
	}

	for i, m := range models {
		if i > 0 || len(enums) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(GenerateCreateTable(m, fks, dialect))
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
func collectPivotTables(models []ModelInfo, dialect string) []string {
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

			srcPKType := GoTypeToSQL(m.PKType, dialect)
			tgtPKType := GoTypeToSQL(modelPK[f.Relation.TargetModel], dialect)

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

// DiffSchema compares desired models against existing migration SQL and generates
// incremental up/down SQL containing only the changes (new tables, new columns).
// existingSQL should be the concatenated up-migration SQL of all prior migrations.
// Returns empty strings if there are no changes.
//
// This is a basic diff approach: it detects new tables and new columns in existing
// tables. It does NOT handle column type changes, renames, or drops.
// Full Atlas integration is planned for a future release.
func DiffSchema(models []ModelInfo, existingSQL string, dialect string) (upSQL, downSQL string) {
	existingTables, existingColumns := parseExistingSchema(existingSQL)
	fks := collectFKs(models)

	var upBuf, downBuf strings.Builder

	// Collect enum types needed for new tables/columns (Postgres only).
	if dialect != "sqlite" {
		newEnums := diffEnums(models, existingSQL)
		for _, e := range newEnums {
			upBuf.WriteString(e)
			upBuf.WriteString("\n\n")
		}
	}

	for _, m := range models {
		if _, exists := existingTables[m.TableName]; !exists {
			// Entirely new table — emit CREATE TABLE.
			if upBuf.Len() > 0 {
				upBuf.WriteString("\n")
			}
			upBuf.WriteString(GenerateCreateTable(m, fks, dialect))

			downBuf.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", quoteIdent(m.TableName)))
		} else {
			// Table exists — check for new columns.
			tableCols := existingColumns[m.TableName]
			for _, f := range columnFields(m.Fields) {
				if tableCols[f.ColumnName] {
					continue
				}
				nullable := strings.HasPrefix(f.GoType, "*")
				sqlType := GoTypeToSQL(f.GoType, dialect)
				if f.IsEnum {
					if dialect == "sqlite" {
						sqlType = "TEXT"
					} else {
						sqlType = quoteIdent(strings.ToLower(f.LocalGoType))
					}
				}
				nullClause := " NOT NULL"
				if nullable {
					nullClause = ""
				}

				ref := ""
				if fks != nil {
					if target, ok := fks[f.ColumnName]; ok {
						ref = fmt.Sprintf(" REFERENCES %s(%s)", quoteIdent(target), quoteIdent("id"))
					}
				}

				enumCheck := ""
				if f.IsEnum && dialect == "sqlite" && len(f.EnumValues) > 0 {
					quoted := make([]string, len(f.EnumValues))
					for i, v := range f.EnumValues {
						quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
					}
					enumCheck = fmt.Sprintf(" CHECK(%s IN (%s))", quoteIdent(f.ColumnName), strings.Join(quoted, ", "))
				}

				upBuf.WriteString(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s%s%s;\n",
					quoteIdent(m.TableName), quoteIdent(f.ColumnName), sqlType, nullClause, enumCheck, ref))
				downBuf.WriteString(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;\n",
					quoteIdent(m.TableName), quoteIdent(f.ColumnName)))
			}
		}
	}

	// Check for new pivot tables.
	pivots := collectPivotTables(models, dialect)
	for _, p := range pivots {
		// Extract table name from "CREATE TABLE "name" ("
		tableName := extractTableName(p)
		if tableName != "" && !existingTables[tableName] {
			if upBuf.Len() > 0 {
				upBuf.WriteString("\n")
			}
			upBuf.WriteString(p)
			downBuf.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", quoteIdent(tableName)))
		}
	}

	return strings.TrimSpace(upBuf.String()), strings.TrimSpace(downBuf.String())
}

// parseExistingSchema extracts table names and their columns from concatenated
// migration SQL. It parses CREATE TABLE statements and ALTER TABLE ADD COLUMN
// statements with a simple line-based approach — no full SQL parser needed.
func parseExistingSchema(sql string) (tables map[string]bool, columns map[string]map[string]bool) {
	tables = make(map[string]bool)
	columns = make(map[string]map[string]bool)

	upper := strings.ToUpper(sql)
	lines := strings.Split(sql, "\n")

	var currentTable string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upperTrimmed := strings.ToUpper(trimmed)

		// Match CREATE TABLE "name" ( or CREATE TABLE name (
		if strings.HasPrefix(upperTrimmed, "CREATE TABLE") {
			name := extractIdentAfter(trimmed, "CREATE TABLE")
			if name != "" {
				tables[name] = true
				columns[name] = make(map[string]bool)
				currentTable = name
			}
			continue
		}

		// End of CREATE TABLE block
		if currentTable != "" && (trimmed == ");" || trimmed == ")") {
			currentTable = ""
			continue
		}

		// Column definition inside CREATE TABLE
		if currentTable != "" {
			col := extractColumnName(trimmed)
			if col != "" {
				columns[currentTable][col] = true
			}
			continue
		}

		// ALTER TABLE "name" ADD COLUMN "col" ...
		if strings.HasPrefix(upperTrimmed, "ALTER TABLE") && strings.Contains(upperTrimmed, "ADD COLUMN") {
			tableName := extractIdentAfter(trimmed, "ALTER TABLE")
			if tableName != "" {
				colPart := trimmed
				idx := strings.Index(strings.ToUpper(colPart), "ADD COLUMN")
				if idx >= 0 {
					rest := strings.TrimSpace(colPart[idx+len("ADD COLUMN"):])
					col := extractLeadingIdent(rest)
					if col != "" {
						if columns[tableName] == nil {
							columns[tableName] = make(map[string]bool)
						}
						columns[tableName][col] = true
					}
				}
			}
		}
	}
	_ = upper // used for string matching context

	return tables, columns
}

// extractIdentAfter extracts a quoted or unquoted identifier after a keyword prefix.
func extractIdentAfter(line, prefix string) string {
	idx := strings.Index(strings.ToUpper(line), strings.ToUpper(prefix))
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(line[idx+len(prefix):])
	return extractLeadingIdent(rest)
}

// extractLeadingIdent extracts a quoted ("name") or unquoted identifier from the start of s.
func extractLeadingIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}
	if s[0] == '"' {
		end := strings.Index(s[1:], `"`)
		if end < 0 {
			return ""
		}
		return s[1 : end+1]
	}
	// Unquoted: take until whitespace or (
	end := strings.IndexAny(s, " \t(,;")
	if end < 0 {
		return s
	}
	return s[:end]
}

// extractColumnName extracts the column name from a CREATE TABLE column definition line.
func extractColumnName(line string) string {
	trimmed := strings.TrimSpace(line)
	// Skip PRIMARY KEY constraints
	if strings.HasPrefix(strings.ToUpper(trimmed), "PRIMARY KEY") {
		return ""
	}
	// Skip comments
	if strings.HasPrefix(trimmed, "--") {
		return ""
	}
	// Skip empty lines
	if trimmed == "" {
		return ""
	}
	// Remove trailing comma
	trimmed = strings.TrimRight(trimmed, ",")
	return extractLeadingIdent(trimmed)
}

// extractTableName extracts a table name from a CREATE TABLE statement string.
func extractTableName(createSQL string) string {
	return extractIdentAfter(createSQL, "CREATE TABLE")
}

// diffEnums returns CREATE TYPE statements for enum types that don't already
// appear in the existing migration SQL.
func diffEnums(models []ModelInfo, existingSQL string) []string {
	seen := map[string]bool{}
	var enums []string
	upper := strings.ToUpper(existingSQL)
	for _, m := range models {
		for _, f := range columnFields(m.Fields) {
			if f.IsEnum && len(f.EnumValues) > 0 {
				name := strings.ToLower(f.LocalGoType)
				if seen[name] {
					continue
				}
				seen[name] = true
				// Check if this enum type already exists in prior migrations.
				if strings.Contains(upper, fmt.Sprintf("CREATE TYPE %s AS ENUM", strings.ToUpper(quoteIdent(name)))) {
					continue
				}
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
