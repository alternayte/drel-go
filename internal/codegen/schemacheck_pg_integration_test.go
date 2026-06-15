//go:build integration

package codegen_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/codegen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckConstraints_EnforceOnPostgres proves generated CHECK constraints,
// including comma-containing IN-lists, actually reject violating rows on a real
// Postgres instance — and that a db:"...,default=" value is applied.
func TestCheckConstraints_EnforceOnPostgres(t *testing.T) {
	ctx := context.Background()
	drv := pgDriver(t)

	models := []codegen.ModelInfo{{
		Name: "Account", TableName: "accounts", PKType: "int",
		Fields: []codegen.FieldInfo{
			{Name: "Age", GoType: "int", ColumnName: "age", IsExported: true,
				CheckExpr: "age >= 0"},
			{Name: "Role", GoType: "string", ColumnName: "role", IsExported: true,
				CheckExpr: "role IN ('admin','user')", Default: "user"},
		},
	}}

	_, err := drv.Exec(ctx, codegen.GenerateSchema(models, "postgres"))
	require.NoError(t, err, "schema should create on Postgres")

	// Valid row: omit role to exercise the DEFAULT, and a non-negative age.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age) VALUES (5)")
	require.NoError(t, err)

	var role string
	row := drv.QueryRow(ctx, "SELECT role FROM accounts WHERE age = 5")
	require.NoError(t, row.Scan(&role))
	assert.Equal(t, "user", role, "default= should populate role when omitted")

	// CHECK on age rejects a negative value.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (-1, 'admin')")
	assert.Error(t, err, "age >= 0 CHECK should reject -1")

	// IN-list CHECK (the comma case) rejects an out-of-set role.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (1, 'wizard')")
	assert.Error(t, err, "role IN ('admin','user') CHECK should reject 'wizard'")

	// Both in-set roles are accepted.
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (1, 'admin')")
	require.NoError(t, err)
	_, err = drv.Exec(ctx, "INSERT INTO accounts (age, role) VALUES (1, 'user')")
	require.NoError(t, err)
}
