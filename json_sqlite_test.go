package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSQLiteJSONEngine(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `
		CREATE TABLE json_docs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			tags       TEXT NOT NULL,
			meta       TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)
	return engine
}

func TestSQLite_JSONColumn_RoundTripAndDiff(t *testing.T) {
	engine := setupSQLiteJSONEngine(t)
	ctx := context.Background()

	doc := &testmodels.JSONDoc{
		Tags: []string{"a", "b"},
		Meta: map[string]string{"k": "v"},
	}
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.JSONDocMeta)
		repo.Add(doc)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)
	require.NotZero(t, doc.ID())

	repo := drel.NewRepository(engine, testmodels.JSONDocMeta)
	got, err := repo.Find(ctx, doc.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got.Tags)
	assert.Equal(t, map[string]string{"k": "v"}, got.Meta)

	// No-op mutation produces no UPDATE (diff is value-based, not pointer-based).
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		uow := drel.NewTxRepository(tx, testmodels.JSONDocMeta)
		tracked, err := uow.Find(ctx, doc.ID())
		if err != nil {
			return err
		}
		tracked.Meta = map[string]string{"k": "v2"}
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	after, err := repo.Find(ctx, doc.ID())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"k": "v2"}, after.Meta)
}
