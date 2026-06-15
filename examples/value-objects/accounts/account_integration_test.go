//go:build integration

package accounts_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alternayte/drel/examples/value-objects/accounts"
)

func newSQLiteEngine(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `CREATE TABLE accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner TEXT NOT NULL,
		balance_amount TEXT NOT NULL,
		balance_currency TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)
	return engine
}

func TestMultiColVO_RoundTrip_SQLite(t *testing.T) {
	engine := newSQLiteEngine(t)
	ctx := context.Background()

	// Insert via UnitOfWork (exercises expanded InsertColumns).
	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, accounts.AccountMeta)
	repo.Add(accounts.NewAccount("alice", accounts.NewMoney(100, "USD")))
	require.NoError(t, uow.SaveChanges(ctx))

	// Read back (exercises generated scan + DrelScanMulti).
	read := drel.NewRepository(engine, accounts.AccountMeta)
	loaded, err := read.Where(accounts.Accounts.BalanceCurrency.Eq("USD")).First(ctx)
	require.NoError(t, err)
	assert.Equal(t, "alice", loaded.Owner())
	assert.Equal(t, 100, loaded.Balance().Amount())
	assert.Equal(t, "USD", loaded.Balance().Currency())

	// Mutate one sub-column, save (exercises per-sub-column diff).
	uow2 := engine.NewUnitOfWork()
	repo2 := drel.NewUoWRepository(uow2, accounts.AccountMeta)
	acct, err := repo2.Find(ctx, loaded.ID())
	require.NoError(t, err)
	acct.SetBalance(accounts.NewMoney(250, "USD"))
	require.NoError(t, uow2.SaveChanges(ctx))

	final, err := read.Find(ctx, loaded.ID())
	require.NoError(t, err)
	assert.Equal(t, 250, final.Balance().Amount())
	assert.Equal(t, "USD", final.Balance().Currency())
}
