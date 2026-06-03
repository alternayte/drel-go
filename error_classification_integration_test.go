//go:build integration

package drel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ErrorClassification confirms SQLSTATE-based classification on
// real Postgres and that the original *pgconn.PgError remains reachable.
func TestIntegration_ErrorClassification(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()
	for _, ddl := range []string{
		`CREATE TABLE p (id SERIAL PRIMARY KEY, name TEXT NOT NULL UNIQUE, age INT CHECK (age >= 0))`,
		`CREATE TABLE c (id SERIAL PRIMARY KEY, p_id INT NOT NULL REFERENCES p(id))`,
		`INSERT INTO p (name, age) VALUES ('alice', 30)`,
	} {
		_, err := engine.Exec(ctx, ddl)
		require.NoError(t, err)
	}

	cases := []struct {
		name string
		sql  string
		want error
	}{
		{"unique", `INSERT INTO p (name) VALUES ('alice')`, drel.ErrUniqueViolation},
		{"not null", `INSERT INTO p (name) VALUES (NULL)`, drel.ErrNotNullViolation},
		{"check", `INSERT INTO p (name, age) VALUES ('bob', -1)`, drel.ErrCheckViolation},
		{"foreign key", `INSERT INTO c (p_id) VALUES (999)`, drel.ErrForeignKeyViolation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Exec(ctx, tc.sql)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.want), "want %v, got %v", tc.want, err)

			// The original pgconn error is still reachable.
			var pgErr *pgconn.PgError
			assert.True(t, errors.As(err, &pgErr), "original *pgconn.PgError should be reachable")
		})
	}
}
