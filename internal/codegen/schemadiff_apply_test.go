package codegen_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/codegen"
	"github.com/alternayte/drel/internal/driver/sqlitedriver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiffSchemas_AppliesToRealSQLite exercises the full generate → diff → apply
// cycle against a real SQLite database, proving the generated CREATE and ALTER
// SQL is valid and the resulting schema behaves as intended.
func TestDiffSchemas_AppliesToRealSQLite(t *testing.T) {
	ctx := context.Background()
	drv, err := sqlitedriver.New(":memory:")
	require.NoError(t, err)
	defer drv.Close()

	usersV1 := codegen.ModelInfo{
		Name: "User", TableName: "users", PKType: "int",
		Fields: []codegen.FieldInfo{
			{Name: "Name", GoType: "string", ColumnName: "name", IsExported: true},
			{Name: "Age", GoType: "int", ColumnName: "age", IsExported: true},
		},
	}
	v1 := []codegen.ModelInfo{usersV1}

	// Apply the initial schema.
	_, err = drv.Exec(ctx, codegen.GenerateSchema(v1, "sqlite"))
	require.NoError(t, err)
	_, err = drv.Exec(ctx, "INSERT INTO users (name, age) VALUES ('alice', 30)")
	require.NoError(t, err)

	// Evolve: add a nullable column + an index on age, and a brand-new table.
	usersV2 := usersV1
	usersV2.Fields = []codegen.FieldInfo{
		{Name: "Name", GoType: "string", ColumnName: "name", IsExported: true},
		{Name: "Age", GoType: "int", ColumnName: "age", IsExported: true, Indexed: true},
		{Name: "Bio", GoType: "*string", ColumnName: "bio", IsExported: true},
	}
	posts := codegen.ModelInfo{
		Name: "Post", TableName: "posts", PKType: "int",
		Fields: []codegen.FieldInfo{
			{Name: "Title", GoType: "string", ColumnName: "title", IsExported: true},
		},
	}
	v2 := []codegen.ModelInfo{usersV2, posts}

	schemaV1 := codegen.BuildSchema(v1, "sqlite")
	schemaV2 := codegen.BuildSchema(v2, "sqlite")

	up, down := codegen.DiffSchemas(schemaV1, schemaV2, "sqlite")
	require.NotEmpty(t, up)
	require.NotEmpty(t, down)

	// Apply the up migration and verify the new shape works.
	_, err = drv.Exec(ctx, up)
	require.NoError(t, err, "up migration should apply cleanly:\n%s", up)

	_, err = drv.Exec(ctx, "UPDATE users SET bio = 'hi' WHERE name = 'alice'")
	require.NoError(t, err, "new bio column should be usable")
	_, err = drv.Exec(ctx, "INSERT INTO posts (title) VALUES ('first')")
	require.NoError(t, err, "new posts table should exist")

	// The age index should now exist.
	row := drv.QueryRow(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_users_age'")
	var n int
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 1, n, "age index should have been created")

	// Apply the down migration and verify the schema reverted.
	_, err = drv.Exec(ctx, down)
	require.NoError(t, err, "down migration should apply cleanly:\n%s", down)

	_, err = drv.Exec(ctx, "INSERT INTO posts (title) VALUES ('x')")
	assert.Error(t, err, "posts table should be gone after down")
	_, err = drv.Exec(ctx, "UPDATE users SET bio = 'y' WHERE name = 'alice'")
	assert.Error(t, err, "bio column should be gone after down")
}

// TestDiffSchemas_NoChange returns empty when schemas match.
func TestDiffSchemas_NoChange(t *testing.T) {
	models := []codegen.ModelInfo{{
		Name: "User", TableName: "users", PKType: "int",
		Fields: []codegen.FieldInfo{{Name: "Name", GoType: "string", ColumnName: "name", IsExported: true}},
	}}
	s := codegen.BuildSchema(models, "postgres")
	up, down := codegen.DiffSchemas(s, s, "postgres")
	assert.Empty(t, up)
	assert.Empty(t, down)
}
