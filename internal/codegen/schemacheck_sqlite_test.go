package codegen_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/codegen"
	"github.com/alternayte/drel/internal/driver/sqlitedriver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckConstraints_EnforceOnSQLite proves generated CHECK constraints,
// including comma-containing IN-lists, reject violating rows on real SQLite, and
// that db:"...,default=" is applied when the column is omitted.
func TestCheckConstraints_EnforceOnSQLite(t *testing.T) {
	ctx := context.Background()
	drv, err := sqlitedriver.New(":memory:")
	require.NoError(t, err)
	defer drv.Close()

	models := []codegen.ModelInfo{{
		Name: "Account", TableName: "accounts", PKType: "int",
		Fields: []codegen.FieldInfo{
			{Name: "Age", GoType: "int", ColumnName: "age", IsExported: true,
				CheckExpr: "age >= 0"},
			{Name: "Role", GoType: "string", ColumnName: "role", IsExported: true,
				CheckExpr: "role IN ('admin','user')", Default: "user"},
		},
	}}

	_, err = drv.Exec(ctx, codegen.GenerateSchema(models, "sqlite"))
	require.NoError(t, err, "schema should create on SQLite")

	// Valid row: omit role to exercise DEFAULT.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age) VALUES (5)")
	require.NoError(t, err)

	var role string
	row := drv.QueryRow(ctx, "SELECT role FROM accounts WHERE age = 5")
	require.NoError(t, row.Scan(&role))
	assert.Equal(t, "user", role, "default= should populate role when omitted")

	// age >= 0 CHECK rejects -1.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (-1, 'admin')")
	assert.Error(t, err, "age >= 0 CHECK should reject -1")

	// IN-list CHECK (the comma case) rejects an out-of-set role.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (1, 'wizard')")
	assert.Error(t, err, "role IN ('admin','user') CHECK should reject 'wizard'")

	// In-set role accepted.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (1, 'admin')")
	require.NoError(t, err)
}
