package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// helper column/table builders ----------------------------------------------

func pgTable(name string, cols ...Column) Table {
	return Table{Name: name, Columns: cols}
}

func TestDiffSchemas_NoChange(t *testing.T) {
	s := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "name", Type: "text", NotNull: true},
		),
	}}
	up, down := DiffSchemas(s, s, "postgres")
	assert.Equal(t, "", up)
	assert.Equal(t, "", down)
}

func TestDiffSchemas_AddTable(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
		Table{
			Name: "posts",
			Columns: []Column{
				{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				{Name: "title", Type: "text", NotNull: true},
			},
			Indexes: []Index{{Name: "idx_posts_title", Columns: []string{"title"}}},
		},
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `CREATE TABLE "posts"`)
	assert.Contains(t, up, `"title" text NOT NULL`)
	assert.Contains(t, up, `CREATE INDEX "idx_posts_title" ON "posts" ("title");`)
	assert.NotContains(t, up, `CREATE TABLE "users"`)
	assert.Contains(t, down, `DROP TABLE IF EXISTS "posts";`)
}

func TestDiffSchemas_DropTable(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
		pgTable("posts", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "title", Type: "text", NotNull: true}),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `DROP TABLE IF EXISTS "posts";`)
	assert.Contains(t, down, `CREATE TABLE "posts"`)
	assert.Contains(t, down, `"title" text NOT NULL`)
}

func TestDiffSchemas_AddColumn_Nullable(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "bio", Type: "text"},
		),
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `ALTER TABLE "users" ADD COLUMN "bio" text;`)
	assert.NotContains(t, up, `"bio" text NOT NULL`)
	assert.Contains(t, down, `ALTER TABLE "users" DROP COLUMN "bio";`)
}

func TestDiffSchemas_AddColumn_NotNull(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "email", Type: "text", NotNull: true},
		),
	}}
	up, _ := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `ALTER TABLE "users" ADD COLUMN "email" text NOT NULL;`)
}

func TestDiffSchemas_DropColumn(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "legacy", Type: "text"},
		),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `ALTER TABLE "users" DROP COLUMN "legacy";`)
	assert.Contains(t, down, `ALTER TABLE "users" ADD COLUMN "legacy" text;`)
}

func TestDiffSchemas_TypeChange_Postgres(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "age", Type: "integer", NotNull: true},
		),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "age", Type: "bigint", NotNull: true},
		),
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `ALTER TABLE "users" ALTER COLUMN "age" TYPE bigint;`)
	assert.Contains(t, down, `ALTER TABLE "users" ALTER COLUMN "age" TYPE integer;`)
}

func TestDiffSchemas_TypeChange_SQLiteWarning(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true},
			Column{Name: "age", Type: "INTEGER", NotNull: true},
		),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true},
			Column{Name: "age", Type: "TEXT", NotNull: true},
		),
	}}
	up, _ := DiffSchemas(old, newS, "sqlite")
	assert.Contains(t, up, `-- WARNING: SQLite cannot ALTER COLUMN TYPE for "users"."age" (INTEGER -> TEXT)`)
}

func TestDiffSchemas_NotNullChange_Postgres(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "bio", Type: "text"},
		),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "bio", Type: "text", NotNull: true},
		),
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `ALTER TABLE "users" ALTER COLUMN "bio" SET NOT NULL;`)
	assert.Contains(t, down, `ALTER TABLE "users" ALTER COLUMN "bio" DROP NOT NULL;`)
}

func TestDiffSchemas_NotNullChange_SQLiteWarning(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true},
			Column{Name: "bio", Type: "TEXT"},
		),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true},
			Column{Name: "bio", Type: "TEXT", NotNull: true},
		),
	}}
	up, _ := DiffSchemas(old, newS, "sqlite")
	assert.Contains(t, up, `-- WARNING: SQLite cannot ALTER COLUMN NOT NULL for "users"."bio"`)
}

func TestDiffSchemas_AddIndex(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "email", Type: "text", NotNull: true},
		),
	}}
	newS := Schema{Tables: []Table{
		{
			Name: "users",
			Columns: []Column{
				{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				{Name: "email", Type: "text", NotNull: true},
			},
			Indexes: []Index{{Name: "idx_users_email", Columns: []string{"email"}}},
		},
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `CREATE INDEX "idx_users_email" ON "users" ("email");`)
	assert.Contains(t, down, `DROP INDEX "idx_users_email";`)
}

func TestDiffSchemas_DropIndex(t *testing.T) {
	old := Schema{Tables: []Table{
		{
			Name: "users",
			Columns: []Column{
				{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				{Name: "email", Type: "text", NotNull: true},
			},
			Indexes: []Index{{Name: "idx_users_email", Columns: []string{"email"}}},
		},
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "email", Type: "text", NotNull: true},
		),
	}}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `DROP INDEX "idx_users_email";`)
	assert.Contains(t, down, `CREATE INDEX "idx_users_email" ON "users" ("email");`)
}

func TestDiffSchemas_UniqueIndex(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "email", Type: "text", NotNull: true},
		),
	}}
	newS := Schema{Tables: []Table{
		{
			Name: "users",
			Columns: []Column{
				{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				{Name: "email", Type: "text", NotNull: true},
			},
			Indexes: []Index{{Name: "uq_users_email", Columns: []string{"email"}, Unique: true}},
		},
	}}
	up, _ := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `CREATE UNIQUE INDEX "uq_users_email" ON "users" ("email");`)
}

func TestDiffSchemas_CompositeIndex(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
			Column{Name: "first", Type: "text", NotNull: true},
			Column{Name: "last", Type: "text", NotNull: true},
		),
	}}
	newS := Schema{Tables: []Table{
		{
			Name: "users",
			Columns: []Column{
				{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				{Name: "first", Type: "text", NotNull: true},
				{Name: "last", Type: "text", NotNull: true},
			},
			Indexes: []Index{{Name: "idx_name", Columns: []string{"first", "last"}}},
		},
	}}
	up, _ := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `CREATE INDEX "idx_name" ON "users" ("first", "last");`)
}

func TestDiffSchemas_AddEnum_Postgres(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true}),
	}}
	newS := Schema{
		Enums: []EnumDef{{Name: "role", Values: []string{"admin", "user"}}},
		Tables: []Table{
			pgTable("users",
				Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				Column{Name: "role", Type: `"role"`, NotNull: true},
			),
		},
	}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `CREATE TYPE "role" AS ENUM ('admin', 'user');`)
	assert.Contains(t, up, `ALTER TABLE "users" ADD COLUMN "role" "role" NOT NULL;`)
	assert.Contains(t, down, `DROP TYPE "role";`)
}

func TestDiffSchemas_AddEnum_SQLiteNoCreateType(t *testing.T) {
	old := Schema{Tables: []Table{
		pgTable("users", Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true}),
	}}
	newS := Schema{Tables: []Table{
		pgTable("users",
			Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true},
			Column{Name: "role", Type: "TEXT", NotNull: true, Check: `"role" IN ('admin', 'user')`},
		),
	}}
	up, _ := DiffSchemas(old, newS, "sqlite")
	assert.NotContains(t, up, "CREATE TYPE")
	assert.Contains(t, up, `ALTER TABLE "users" ADD COLUMN "role" TEXT NOT NULL CHECK("role" IN ('admin', 'user'));`)
}

// BuildSchema-driven integration: full round of model -> schema -> diff.
func TestBuildSchema_IndexesFromTags(t *testing.T) {
	m := ModelInfo{
		Name: "User", PKType: "int", TableName: "users",
		Fields: []FieldInfo{
			{Name: "email", GoType: "string", ColumnName: "email", Unique: true},
			{Name: "age", GoType: "int", ColumnName: "age", Indexed: true},
			{Name: "first", GoType: "string", ColumnName: "first", IndexName: "idx_name"},
			{Name: "last", GoType: "string", ColumnName: "last", IndexName: "idx_name"},
			{Name: "score", GoType: "int", ColumnName: "score", CheckExpr: "score >= 0"},
		},
	}
	s := BuildSchema([]ModelInfo{m}, "postgres")
	tbl := s.Tables[0]

	// Indexes sorted by name: idx_name, idx_users_age, uq_users_email
	assert.Equal(t, []Index{
		{Name: "idx_name", Columns: []string{"first", "last"}},
		{Name: "idx_users_age", Columns: []string{"age"}},
		{Name: "uq_users_email", Columns: []string{"email"}, Unique: true},
	}, tbl.Indexes)

	// Check constraint surfaces on the score column.
	var score Column
	for _, c := range tbl.Columns {
		if c.Name == "score" {
			score = c
		}
	}
	assert.Equal(t, "score >= 0", score.Check)
}

func TestBuildSchema_DefaultAndTypeOverride(t *testing.T) {
	m := ModelInfo{
		Name: "User", PKType: "int", TableName: "users",
		Fields: []FieldInfo{
			{Name: "role", GoType: "string", ColumnName: "role", Default: "user"},
			{Name: "age", GoType: "int", ColumnName: "age", Default: "0"},
			{Name: "meta", GoType: "string", ColumnName: "meta", TypeOverride: "jsonb"},
			{Name: "ts", GoType: "time.Time", ColumnName: "ts", Default: "now()"},
		},
	}
	cols := func(s Schema) map[string]Column {
		out := map[string]Column{}
		for _, c := range s.Tables[0].Columns {
			out[c.Name] = c
		}
		return out
	}

	pg := cols(BuildSchema([]ModelInfo{m}, "postgres"))
	// String default is single-quoted; numeric default is bare; SQL-call default is bare.
	assert.Equal(t, "'user'", pg["role"].Default)
	assert.Equal(t, "0", pg["age"].Default)
	assert.Equal(t, "now()", pg["ts"].Default)
	// type= override replaces the inferred type.
	assert.Equal(t, "jsonb", pg["meta"].Type)

	sl := cols(BuildSchema([]ModelInfo{m}, "sqlite"))
	assert.Equal(t, "'user'", sl["role"].Default)
	assert.Equal(t, "0", sl["age"].Default)
	assert.Equal(t, "jsonb", sl["meta"].Type)
}

func TestDiffSchemas_GrowStringEnum_Postgres(t *testing.T) {
	old := Schema{
		Enums: []EnumDef{{Name: "role", Values: []string{"admin", "user"}, BaseType: "string"}},
		Tables: []Table{
			pgTable("users",
				Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				Column{Name: "role", Type: `"role"`, NotNull: true},
			),
		},
	}
	newS := Schema{
		Enums: []EnumDef{{Name: "role", Values: []string{"admin", "user", "moderator"}, BaseType: "string"}},
		Tables: []Table{
			pgTable("users",
				Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
				Column{Name: "role", Type: `"role"`, NotNull: true},
			),
		},
	}
	up, down := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, `ALTER TYPE "role" ADD VALUE 'moderator';`)
	// down cannot trivially drop an enum value — must warn, not silently no-op.
	assert.Contains(t, down, "WARNING")
	assert.NotEqual(t, "", up)
}

func TestDiffSchemas_ShrinkStringEnum_Postgres_Warns(t *testing.T) {
	old := Schema{
		Enums: []EnumDef{{Name: "role", Values: []string{"admin", "user", "guest"}, BaseType: "string"}},
		Tables: []Table{pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true})},
	}
	newS := Schema{
		Enums: []EnumDef{{Name: "role", Values: []string{"admin", "user"}, BaseType: "string"}},
		Tables: []Table{pgTable("users", Column{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true})},
	}
	up, _ := DiffSchemas(old, newS, "postgres")
	assert.Contains(t, up, "WARNING")
	assert.Contains(t, up, "guest")
}

func TestDiffSchemas_GrowStringEnum_SQLiteNoAlterType(t *testing.T) {
	// SQLite has no CREATE TYPE / ALTER TYPE; string enums never appear in
	// s.Enums for SQLite, so growing one yields no ALTER TYPE here.
	old := Schema{Tables: []Table{pgTable("users", Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true})}}
	newS := Schema{Tables: []Table{pgTable("users", Column{Name: "id", Type: "INTEGER PRIMARY KEY AUTOINCREMENT", NotNull: true, PK: true})}}
	up, _ := DiffSchemas(old, newS, "sqlite")
	assert.NotContains(t, up, "ALTER TYPE")
}

func TestDiffSchemas_GrowIntEnum_ProducesMigration(t *testing.T) {
	v1 := []ModelInfo{{
		Name: "Ticket", PKType: "int", TableName: "tickets", Fields: []FieldInfo{
			{Name: "priority", GoType: "tickets.Priority", ColumnName: "priority", LocalGoType: "Priority",
				IsEnum: true, EnumIsInt: true, EnumBaseType: "int", EnumValues: []string{"0", "1"}},
		},
	}}
	v2 := []ModelInfo{{
		Name: "Ticket", PKType: "int", TableName: "tickets", Fields: []FieldInfo{
			{Name: "priority", GoType: "tickets.Priority", ColumnName: "priority", LocalGoType: "Priority",
				IsEnum: true, EnumIsInt: true, EnumBaseType: "int", EnumValues: []string{"0", "1", "2"}},
		},
	}}

	for _, dialect := range []string{"postgres", "sqlite"} {
		up, _ := DiffSchemas(BuildSchema(v1, dialect), BuildSchema(v2, dialect), dialect)
		assert.NotEqual(t, "", up, "growing an int enum must not produce an empty migration on %s", dialect)
		// The migration references the changed CHECK value set.
		assert.Contains(t, up, "2", "migration should reference the newly added enum value on %s", dialect)
	}
}


