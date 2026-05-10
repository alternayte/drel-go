package drel_test

import (
	"testing"

	"github.com/alternayte/drel/internal/dialect"
	"github.com/alternayte/drel/internal/dialect/postgres"
	"github.com/stretchr/testify/assert"
)

func TestPostgres_BuildInsert(t *testing.T) {
	pg := postgres.New()
	result := pg.BuildInsert("users", []string{"name", "email", "age"}, []any{"Alice", "alice@test.com", 30}, []string{"id", "created_at", "updated_at"})
	assert.Equal(t, `INSERT INTO "users" ("name", "email", "age") VALUES ($1, $2, $3) RETURNING "id", "created_at", "updated_at"`, result.SQL)
	assert.Equal(t, []any{"Alice", "alice@test.com", 30}, result.Args)
}

func TestPostgres_BuildInsert_NoReturning(t *testing.T) {
	pg := postgres.New()
	result := pg.BuildInsert("logs", []string{"message"}, []any{"hello"}, nil)
	assert.Equal(t, `INSERT INTO "logs" ("message") VALUES ($1)`, result.SQL)
	assert.Equal(t, []any{"hello"}, result.Args)
}

func TestPostgres_BuildUpdate_SingleField(t *testing.T) {
	pg := postgres.New()
	result := pg.BuildUpdate("users", []dialect.ColumnValue{{Column: "name", Value: "Bob"}}, "id", 1)
	assert.Equal(t, `UPDATE "users" SET "name" = $1 WHERE "id" = $2`, result.SQL)
	assert.Equal(t, []any{"Bob", 1}, result.Args)
}

func TestPostgres_BuildUpdate_MultipleFields(t *testing.T) {
	pg := postgres.New()
	result := pg.BuildUpdate("users", []dialect.ColumnValue{{Column: "name", Value: "Bob"}, {Column: "age", Value: 31}}, "id", 42)
	assert.Equal(t, `UPDATE "users" SET "name" = $1, "age" = $2 WHERE "id" = $3`, result.SQL)
	assert.Equal(t, []any{"Bob", 31, 42}, result.Args)
}

func TestPostgres_BuildDelete(t *testing.T) {
	pg := postgres.New()
	result := pg.BuildDelete("users", "id", 99)
	assert.Equal(t, `DELETE FROM "users" WHERE "id" = $1`, result.SQL)
	assert.Equal(t, []any{99}, result.Args)
}
