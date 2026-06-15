//go:build integration

package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	vomodels "github.com/alternayte/drel/examples/value-objects/models"
	"github.com/alternayte/drel/dreltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func voRoundTrip(t *testing.T, engine *drel.Engine, dialect string) {
	ctx := context.Background()

	email, err := vomodels.NewEmail("Alice@Example.com")
	require.NoError(t, err)

	// Insert.
	var id int
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, vomodels.AccountMeta)
		a := vomodels.NewAccount(email, vomodels.NewCents(1000))
		repo.Add(a)
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		id = a.ID()
		return nil
	})
	require.NoError(t, err)

	// Prove the int64-backed VO stored as an integer, not text: arithmetic in SQL
	// must work (it would error/compare lexically on a TEXT column).
	row := engine.QueryRow(ctx,
		"SELECT balance + 1 FROM accounts WHERE id = "+voPlaceholder(dialect, 1), id)
	var balPlus int64
	require.NoError(t, row.Scan(&balPlus))
	assert.Equal(t, int64(1001), balPlus, "balance must be stored as an integer column")

	// Query by VO equality (Where(Accounts.Email.Eq(vo))).
	repo := drel.NewRepository(engine, vomodels.AccountMeta)
	got, err := repo.Where(vomodels.Accounts.Email.Eq(email)).First(ctx)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", got.Email().String())
	assert.Equal(t, int64(1000), got.Balance().Int64())

	// Mutate via a tracked unit of work and SaveChanges; diff must fire.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		trepo := drel.NewTxRepository(tx, vomodels.AccountMeta)
		acc, err := trepo.Find(ctx, id)
		if err != nil {
			return err
		}
		acc.SetBalance(vomodels.NewCents(2500))
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	after, err := drel.NewRepository(engine, vomodels.AccountMeta).Find(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), after.Balance().Int64())
}

// voPlaceholder returns the dialect-correct positional placeholder for a raw query.
func voPlaceholder(dialect string, n int) string {
	if dialect == "sqlite" {
		return "?"
	}
	return "$1"
}

func TestIntegration_ValueObject_RoundTrip_Postgres(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `CREATE TABLE accounts (
		id         SERIAL PRIMARY KEY,
		email      text NOT NULL,
		balance    bigint NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		UNIQUE(email)
	)`)
	require.NoError(t, err)
	voRoundTrip(t, engine, "postgres")
}

func TestIntegration_ValueObject_RoundTrip_SQLite(t *testing.T) {
	engine := dreltest.NewSQLite(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `CREATE TABLE accounts (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		email      TEXT NOT NULL,
		balance    INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(email)
	)`)
	require.NoError(t, err)
	voRoundTrip(t, engine, "sqlite")
}
