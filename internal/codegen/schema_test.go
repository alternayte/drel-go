package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			assert.Equal(t, tt.sqlType, GoTypeToSQL(tt.goType))
		})
	}
}

func TestGoTypeToSQL_Pointer(t *testing.T) {
	assert.Equal(t, "text", GoTypeToSQL("*string"))
	assert.Equal(t, "integer", GoTypeToSQL("*int"))
}

func TestGoTypeToSQL_Unknown(t *testing.T) {
	assert.Equal(t, "text", GoTypeToSQL("custom.Type"))
}

func TestGenerateCreateTable_BasicModel(t *testing.T) {
	m := ModelInfo{
		Name: "User", PkgName: "users", PKType: "int", TableName: "users",
		Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "email", GoType: "string", ColumnName: "email"},
		},
	}
	sql := GenerateCreateTable(m, nil)

	assert.Contains(t, sql, "CREATE TABLE users (")
	assert.Contains(t, sql, "id SERIAL PRIMARY KEY")
	assert.Contains(t, sql, "name text NOT NULL")
	assert.Contains(t, sql, "email text NOT NULL")
	assert.Contains(t, sql, "created_at timestamptz NOT NULL DEFAULT NOW()")
	assert.Contains(t, sql, "updated_at timestamptz NOT NULL DEFAULT NOW()")
}

func TestGenerateCreateTable_BigintPK(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int64", TableName: "posts",
		Fields: []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}}}
	sql := GenerateCreateTable(m, nil)
	assert.Contains(t, sql, "id BIGSERIAL PRIMARY KEY")
}

func TestGenerateCreateTable_Traits(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int", TableName: "posts",
		Fields: []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}},
		HasSoftDelete: true, HasVersioned: true, HasAudit: true}
	sql := GenerateCreateTable(m, nil)

	assert.Contains(t, sql, "deleted_at timestamptz")
	assert.NotContains(t, sql, "deleted_at timestamptz NOT NULL")
	assert.Contains(t, sql, "version integer NOT NULL DEFAULT 1")
	assert.Contains(t, sql, "created_by text")
	assert.Contains(t, sql, "updated_by text")
}

func TestGenerateCreateTable_Nullable(t *testing.T) {
	m := ModelInfo{Name: "Profile", PKType: "int", TableName: "profiles",
		Fields: []FieldInfo{{Name: "bio", GoType: "*string", ColumnName: "bio"}}}
	sql := GenerateCreateTable(m, nil)

	assert.Contains(t, sql, "bio text")
	assert.NotContains(t, sql, "bio text NOT NULL")
}

func TestGenerateSchema_MultipleModels(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users",
			Fields: []FieldInfo{{Name: "name", GoType: "string", ColumnName: "name"}}},
		{Name: "Post", PKType: "int", TableName: "posts",
			Fields: []FieldInfo{{Name: "title", GoType: "string", ColumnName: "title"}}},
	}
	sql := GenerateSchema(models)
	assert.Contains(t, sql, "CREATE TABLE users")
	assert.Contains(t, sql, "CREATE TABLE posts")
}

func TestGenerateSchema_WithEnum(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", PKType: "int", TableName: "users", Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name"},
			{Name: "role", GoType: "users.Role", ColumnName: "role", LocalGoType: "Role",
				IsEnum: true, EnumValues: []string{"admin", "user"}},
		}},
	}
	sql := GenerateSchema(models)

	assert.Contains(t, sql, "CREATE TYPE role AS ENUM ('admin', 'user');")
	assert.Contains(t, sql, "role role NOT NULL")
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
	sql := GenerateSchema(models)

	assert.Contains(t, sql, "author_id integer NOT NULL REFERENCES users(id)")
}

func TestGenerateCreateTable_NoFK_WhenNilMap(t *testing.T) {
	m := ModelInfo{Name: "Post", PKType: "int", TableName: "posts", Fields: []FieldInfo{
		{Name: "authorID", GoType: "int", ColumnName: "author_id"},
	}}
	sql := GenerateCreateTable(m, nil)

	assert.Contains(t, sql, "author_id integer NOT NULL")
	assert.NotContains(t, sql, "REFERENCES")
}

func TestGenerateDropSchema_ReverseOrder(t *testing.T) {
	models := []ModelInfo{
		{Name: "User", TableName: "users"},
		{Name: "Post", TableName: "posts"},
	}
	sql := GenerateDropSchema(models)

	postsIdx := strings.Index(sql, "DROP TABLE IF EXISTS posts")
	usersIdx := strings.Index(sql, "DROP TABLE IF EXISTS users")
	assert.Less(t, postsIdx, usersIdx)
}
