package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// GoTypeToSQL — Postgres (existing behavior preserved)
// ---------------------------------------------------------------------------

func TestGoTypeToSQL(t *testing.T) {
	tests := []struct {
		goType  string
		sqlType string
	}{
		{"int", "integer"},
		{"int8", "smallint"},
		{"int16", "smallint"},
		{"int32", "integer"},
		{"int64", "bigint"},
		{"string", "text"},
		{"bool", "boolean"},
		{"float32", "real"},
		{"float64", "double precision"},
		{"time.Time", "timestamptz"},
		{"github.com/google/uuid.UUID", "uuid"},
		{"uuid.UUID", "uuid"},
	}
	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			assert.Equal(t, tt.sqlType, GoTypeToSQL(tt.goType, "postgres"))
		})
	}
}

func TestGoTypeToSQL_Pointer(t *testing.T) {
	assert.Equal(t, "text", GoTypeToSQL("*string", "postgres"))
	assert.Equal(t, "integer", GoTypeToSQL("*int", "postgres"))
}

func TestGoTypeToSQL_Unknown(t *testing.T) {
	assert.Equal(t, "text", GoTypeToSQL("custom.Type", "postgres"))
}

// ---------------------------------------------------------------------------
// GoTypeToSQL — SQLite
// ---------------------------------------------------------------------------

func TestGoTypeToSQL_SQLite(t *testing.T) {
	tests := []struct {
		goType  string
		sqlType string
	}{
		{"int", "INTEGER"},
		{"int8", "INTEGER"},
		{"int16", "INTEGER"},
		{"int32", "INTEGER"},
		{"int64", "INTEGER"},
		{"uint", "INTEGER"},
		{"uint8", "INTEGER"},
		{"uint16", "INTEGER"},
		{"uint32", "INTEGER"},
		{"uint64", "INTEGER"},
		{"string", "TEXT"},
		{"bool", "INTEGER"},
		{"float32", "REAL"},
		{"float64", "REAL"},
		{"time.Time", "DATETIME"},
		{"github.com/google/uuid.UUID", "TEXT"},
		{"uuid.UUID", "TEXT"},
	}
	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			assert.Equal(t, tt.sqlType, GoTypeToSQL(tt.goType, "sqlite"))
		})
	}
}

func TestGoTypeToSQL_SQLite_Pointer(t *testing.T) {
	assert.Equal(t, "TEXT", GoTypeToSQL("*string", "sqlite"))
	assert.Equal(t, "INTEGER", GoTypeToSQL("*int", "sqlite"))
}

func TestGoTypeToSQL_SQLite_Unknown(t *testing.T) {
	assert.Equal(t, "TEXT", GoTypeToSQL("custom.Type", "sqlite"))
}

// ---------------------------------------------------------------------------
// GenerateCreateTable — Postgres
// ---------------------------------------------------------------------------

func TestGenerateCreateTable_BasicModel(t *testing.T) {
	m := ModelInfo{
		Name: "User", PkgName: "users", PKType: "int", TableName: "users",
		Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "email", GoType: "string", ColumnName: "email"},
		},
	}
	sql := GenerateCreateTable(m, nil, "postgres")

	assert.Contains(t, sql, `CREATE TABLE "users" (`)
	assert.Contains(t, sql, `"id" SERIAL PRIMARY KEY`)
	assert.Contains(t, sql, `"name" text NOT NULL`)
	assert.Contains(t, sql, `"email" text NOT NULL`)
	assert.Contains(t, sql, `"created_at" timestamptz NOT NULL DEFAULT NOW()`)
	assert.Contains(t, sql, `"updated_at" timestamptz NOT NULL DEFAULT NOW()`)
}

func TestGenerateCreateTable_BigintPK(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int64", TableName: "posts",
		Fields: []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}}}
	sql := GenerateCreateTable(m, nil, "postgres")
	assert.Contains(t, sql, `"id" BIGSERIAL PRIMARY KEY`)
}

func TestGenerateCreateTable_Traits(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int", TableName: "posts",
		Fields:        []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}},
		HasSoftDelete: true, HasVersioned: true, HasAudit: true}
	sql := GenerateCreateTable(m, nil, "postgres")

	assert.Contains(t, sql, `"deleted_at" timestamptz`)
	assert.NotContains(t, sql, `"deleted_at" timestamptz NOT NULL`)
	assert.Contains(t, sql, `"version" integer NOT NULL DEFAULT 1`)
	assert.Contains(t, sql, `"created_by" text`)
	assert.Contains(t, sql, `"updated_by" text`)
}

func TestGenerateCreateTable_Nullable(t *testing.T) {
	m := ModelInfo{Name: "Profile", PKType: "int", TableName: "profiles",
		Fields: []FieldInfo{{Name: "bio", GoType: "*string", ColumnName: "bio"}}}
	sql := GenerateCreateTable(m, nil, "postgres")

	assert.Contains(t, sql, `"bio" text`)
	assert.NotContains(t, sql, `"bio" text NOT NULL`)
}

// ---------------------------------------------------------------------------
// GenerateCreateTable — SQLite
// ---------------------------------------------------------------------------

func TestGenerateCreateTable_SQLite_BasicModel(t *testing.T) {
	m := ModelInfo{
		Name: "User", PkgName: "users", PKType: "int", TableName: "users",
		Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "email", GoType: "string", ColumnName: "email"},
		},
	}
	sql := GenerateCreateTable(m, nil, "sqlite")

	assert.Contains(t, sql, `CREATE TABLE "users" (`)
	assert.Contains(t, sql, `"id" INTEGER PRIMARY KEY AUTOINCREMENT`)
	assert.Contains(t, sql, `"name" TEXT NOT NULL`)
	assert.Contains(t, sql, `"email" TEXT NOT NULL`)
	assert.Contains(t, sql, `"created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`)
	assert.Contains(t, sql, `"updated_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`)
	assert.NotContains(t, sql, "NOW()")
	assert.NotContains(t, sql, "timestamptz")
}

func TestGenerateCreateTable_SQLite_BigintPK(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int64", TableName: "posts",
		Fields: []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}}}
	sql := GenerateCreateTable(m, nil, "sqlite")
	// SQLite uses INTEGER PRIMARY KEY AUTOINCREMENT for all integer types.
	assert.Contains(t, sql, `"id" INTEGER PRIMARY KEY AUTOINCREMENT`)
	assert.NotContains(t, sql, "BIGSERIAL")
}

func TestGenerateCreateTable_SQLite_Traits(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int", TableName: "posts",
		Fields:        []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}},
		HasSoftDelete: true, HasVersioned: true, HasAudit: true}
	sql := GenerateCreateTable(m, nil, "sqlite")

	assert.Contains(t, sql, `"deleted_at" DATETIME`)
	assert.NotContains(t, sql, `"deleted_at" DATETIME NOT NULL`)
	assert.Contains(t, sql, `"version" INTEGER NOT NULL DEFAULT 1`)
	assert.Contains(t, sql, `"created_by" TEXT`)
	assert.Contains(t, sql, `"updated_by" TEXT`)
}

func TestGenerateCreateTable_SQLite_UUIDPK(t *testing.T) {
	m := ModelInfo{Name: "Widget", PKType: "uuid.UUID", TableName: "widgets",
		Fields: []FieldInfo{{Name: "name", GoType: "string", ColumnName: "name"}}}
	sql := GenerateCreateTable(m, nil, "sqlite")
	assert.Contains(t, sql, `"id" TEXT PRIMARY KEY`)
	assert.NotContains(t, sql, "AUTOINCREMENT")
}

func TestGenerateCreateTable_SQLite_Enum(t *testing.T) {
	m := ModelInfo{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
		{Name: "role", GoType: "users.Role", ColumnName: "role", LocalGoType: "Role",
			IsEnum: true, EnumValues: []string{"admin", "user"}},
	}}
	sql := GenerateCreateTable(m, nil, "sqlite")

	assert.Contains(t, sql, `"role" TEXT NOT NULL CHECK("role" IN ('admin', 'user'))`)
	assert.NotContains(t, sql, "CREATE TYPE")
}

// ---------------------------------------------------------------------------
// GenerateSchema — Postgres
// ---------------------------------------------------------------------------

func TestGenerateSchema_MultipleModels(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users",
			Fields: []FieldInfo{{Name: "name", GoType: "string", ColumnName: "name"}}},
		{Name: "Post", PKType: "int", TableName: "posts",
			Fields: []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}}},
	}
	sql := GenerateSchema(models, "postgres")
	assert.Contains(t, sql, `CREATE TABLE "users"`)
	assert.Contains(t, sql, `CREATE TABLE "posts"`)
}

func TestGenerateSchema_WithEnum(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "role", GoType: "users.Role", ColumnName: "role", LocalGoType: "Role",
				IsEnum: true, EnumValues: []string{"admin", "user"}},
		}},
	}
	sql := GenerateSchema(models, "postgres")

	assert.Contains(t, sql, `CREATE TYPE "role" AS ENUM ('admin', 'user');`)
	assert.Contains(t, sql, `"role" "role" NOT NULL`)
}

func TestGenerateSchema_BelongsToFK(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
		}},
		{Name: "Post", PKType: "int", TableName: "posts", Fields: []FieldInfo{
			{Name: "title", GoType: "string", ColumnName: "title"},
			{Name: "authorID", GoType: "int", ColumnName: "author_id"},
			{Name: "author", GoType: "*users.User", RelTag: "belongs_to,fk=author_id",
				Relation: &RelationFieldInfo{Type: "belongs_to", FK: "author_id", TargetModel: "User"}},
		}},
	}
	sql := GenerateSchema(models, "postgres")

	assert.Contains(t, sql, `"author_id" integer NOT NULL REFERENCES "users"("id")`)
}

func TestGenerateCreateTable_NoFK_WhenNilMap(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int", TableName: "posts", Fields: []FieldInfo{
		{Name: "authorID", GoType: "int", ColumnName: "author_id"},
	}}
	sql := GenerateCreateTable(m, nil, "postgres")

	assert.Contains(t, sql, `"author_id" integer NOT NULL`)
	assert.NotContains(t, sql, "REFERENCES")
}

func TestGenerateDropSchema_ReverseOrder(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", TableName: "users"},
		{Name: "Post", TableName: "posts"},
	}
	sql := GenerateDropSchema(models)

	postsIdx := strings.Index(sql, `DROP TABLE IF EXISTS "posts"`)
	usersIdx := strings.Index(sql, `DROP TABLE IF EXISTS "users"`)
	assert.Greater(t, postsIdx, -1)
	assert.Greater(t, usersIdx, -1)
	assert.Less(t, postsIdx, usersIdx)
}

func TestGenerateSchema_EnumEscaping(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
			{Name: "status", GoType: "string", ColumnName: "status", LocalGoType: "Status",
				IsEnum: true, EnumValues: []string{"it's", "normal"}},
		}},
	}
	sql := GenerateSchema(models, "postgres")
	assert.Contains(t, sql, "'it''s'")
}

func TestGenerateSchema_ManyToManyPivotTable(t *testing.T) {
	models := []ModelInfo{
		{
			Name: "Author", PkgName: "models", PKType: "int", TableName: "authors",
			Fields: []FieldInfo{
				{Name: "name", GoType: "string", ColumnName: "name", LocalGoType: "string"},
				{Name: "tags", GoType: "[]*Tag", Relation: &RelationFieldInfo{
					Type: "many_to_many", FK: "author_id", JoinTable: "author_tags",
					RefColumn: "tag_id", TargetModel: "Tag",
				}},
			},
		},
		{
			Name: "Tag", PkgName: "models", PKType: "int", TableName: "tags",
			Fields: []FieldInfo{
				{Name: "name", GoType: "string", ColumnName: "name", LocalGoType: "string"},
			},
		},
	}

	schema := GenerateSchema(models, "postgres")

	assert.Contains(t, schema, `CREATE TABLE "author_tags"`)
	assert.Contains(t, schema, `"author_id" integer NOT NULL REFERENCES "authors"("id")`)
	assert.Contains(t, schema, `"tag_id" integer NOT NULL REFERENCES "tags"("id")`)
	assert.Contains(t, schema, `PRIMARY KEY ("author_id", "tag_id")`)
}

func TestGenerateSchema_ManyToManyDeduplication(t *testing.T) {
	models := []ModelInfo{
		{
			Name: "Author", PkgName: "models", PKType: "int", TableName: "authors",
			Fields: []FieldInfo{
				{Name: "tags", GoType: "[]*Tag", Relation: &RelationFieldInfo{
					Type: "many_to_many", FK: "author_id", JoinTable: "author_tags",
					RefColumn: "tag_id", TargetModel: "Tag",
				}},
			},
		},
		{
			Name: "Tag", PkgName: "models", PKType: "int", TableName: "tags",
			Fields: []FieldInfo{
				{Name: "authors", GoType: "[]*Author", Relation: &RelationFieldInfo{
					Type: "many_to_many", FK: "tag_id", JoinTable: "author_tags",
					RefColumn: "author_id", TargetModel: "Author",
				}},
			},
		},
	}

	schema := GenerateSchema(models, "postgres")

	count := strings.Count(schema, `CREATE TABLE "author_tags"`)
	assert.Equal(t, 1, count)
}

// ---------------------------------------------------------------------------
// GenerateSchema — SQLite
// ---------------------------------------------------------------------------

func TestGenerateSchema_SQLite_NoCreateType(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "role", GoType: "users.Role", ColumnName: "role", LocalGoType: "Role",
				IsEnum: true, EnumValues: []string{"admin", "user"}},
		}},
	}
	sql := GenerateSchema(models, "sqlite")

	// No CREATE TYPE for SQLite.
	assert.NotContains(t, sql, "CREATE TYPE")
	// Enum handled via CHECK constraint.
	assert.Contains(t, sql, `CHECK("role" IN ('admin', 'user'))`)
}

func TestGenerateSchema_SQLite_FullSchema(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "email", GoType: "string", ColumnName: "email"},
		}},
		{Name: "Post", PKType: "int64", TableName: "posts", Fields: []FieldInfo{
			{Name: "title", GoType: "string", ColumnName: "title"},
			{Name: "views", GoType: "int", ColumnName: "views"},
		}},
	}
	sql := GenerateSchema(models, "sqlite")

	assert.Contains(t, sql, `"id" INTEGER PRIMARY KEY AUTOINCREMENT`)
	assert.Contains(t, sql, `"name" TEXT NOT NULL`)
	assert.Contains(t, sql, `"views" INTEGER NOT NULL`)
	assert.Contains(t, sql, `CURRENT_TIMESTAMP`)
	assert.NotContains(t, sql, "NOW()")
	assert.NotContains(t, sql, "SERIAL")
	assert.NotContains(t, sql, "timestamptz")
}

func TestGenerateSchema_SQLite_ManyToMany(t *testing.T) {
	models := []ModelInfo{
		{Name: "Author", PkgName: "models", PKType: "int", TableName: "authors",
			Fields: []FieldInfo{
				{Name: "name", GoType: "string", ColumnName: "name"},
				{Name: "tags", GoType: "[]*Tag", Relation: &RelationFieldInfo{
					Type: "many_to_many", FK: "author_id", JoinTable: "author_tags",
					RefColumn: "tag_id", TargetModel: "Tag",
				}},
			}},
		{Name: "Tag", PkgName: "models", PKType: "int", TableName: "tags",
			Fields: []FieldInfo{
				{Name: "name", GoType: "string", ColumnName: "name"},
			}},
	}
	sql := GenerateSchema(models, "sqlite")

	assert.Contains(t, sql, `CREATE TABLE "author_tags"`)
	assert.Contains(t, sql, `"author_id" INTEGER NOT NULL REFERENCES "authors"("id")`)
	assert.Contains(t, sql, `"tag_id" INTEGER NOT NULL REFERENCES "tags"("id")`)
}

func TestGenerateCreateTable_VOBaseType(t *testing.T) {
	m := ModelInfo{
		Name: "Account", PKType: "int", TableName: "accounts",
		Fields: []FieldInfo{
			{Name: "email", GoType: "models.Email", ColumnName: "email", LocalGoType: "Email", IsVO: true, VOBaseType: "string"},
			{Name: "balance", GoType: "models.Cents", ColumnName: "balance", LocalGoType: "Cents", IsVO: true, VOBaseType: "int64"},
		},
	}

	pg := GenerateCreateTable(m, nil, "postgres")
	assert.Contains(t, pg, `"email" text NOT NULL`)
	assert.Contains(t, pg, `"balance" bigint NOT NULL`)
	assert.NotContains(t, pg, `"balance" text`)

	lite := GenerateCreateTable(m, nil, "sqlite")
	assert.Contains(t, lite, `"balance" INTEGER NOT NULL`)
	assert.NotContains(t, lite, `"balance" TEXT`)
}

func TestCreateTable_UUIDPK_NoAutoGen(t *testing.T) {
	m := ModelInfo{
		Name: "Account", PKType: "uuid.UUID", PKTypePkg: "github.com/google/uuid",
		TableName: "accounts",
		Fields:    []FieldInfo{{Name: "Name", GoType: "string", ColumnName: "name"}},
	}

	pg := GenerateCreateTable(m, nil, "postgres")
	if !strings.Contains(pg, `"id" uuid PRIMARY KEY`) {
		t.Fatalf("postgres: expected uuid PK, got:\n%s", pg)
	}
	// Extract the "id" PK line and assert it has no SERIAL or DEFAULT on the PK
	// itself. (DEFAULT is valid on trait columns like created_at; we only
	// care the PK column is free of auto-generation clauses.)
	for _, line := range strings.Split(pg, "\n") {
		if strings.Contains(line, `"id"`) {
			if strings.Contains(line, "SERIAL") || strings.Contains(line, "DEFAULT") {
				t.Fatalf("postgres: app-assigned uuid PK must have no SERIAL/DEFAULT on id line:\n%s", line)
			}
			break
		}
	}

	lite := GenerateCreateTable(m, nil, "sqlite")
	if !strings.Contains(lite, `"id" TEXT PRIMARY KEY`) {
		t.Fatalf("sqlite: expected TEXT PK, got:\n%s", lite)
	}
	if strings.Contains(lite, "AUTOINCREMENT") {
		t.Fatalf("sqlite: app-assigned uuid PK must have no AUTOINCREMENT:\n%s", lite)
	}
}
