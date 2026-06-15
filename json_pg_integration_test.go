//go:build integration

package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJSONDocsTable(t *testing.T, engine *drel.Engine) {
	t.Helper()
	ctx := context.Background()
	_, err := engine.Exec(ctx, `
		CREATE TABLE json_docs (
			id         SERIAL PRIMARY KEY,
			tags       JSONB NOT NULL,
			meta       JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)
}

func TestIntegration_JSONColumn_RoundTripAndDiff(t *testing.T) {
	engine := setupTestDB(t)
	setupJSONDocsTable(t, engine)
	ctx := context.Background()

	doc := &testmodels.JSONDoc{
		Tags: []string{"go", "orm"},
		Meta: map[string]string{"env": "test"},
	}

	// Insert via the tracked write path.
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.JSONDocMeta)
		repo.Add(doc)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)
	require.NotZero(t, doc.ID())

	// Read back through scan (jsonb -> []byte -> JSON.Scan -> Go value).
	repo := drel.NewRepository(engine, testmodels.JSONDocMeta)
	got, err := repo.Find(ctx, doc.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "orm"}, got.Tags)
	assert.Equal(t, map[string]string{"env": "test"}, got.Meta)

	// Mutate and confirm the diff fires a JSON-aware UPDATE.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		uow := drel.NewTxRepository(tx, testmodels.JSONDocMeta)
		tracked, err := uow.Find(ctx, doc.ID())
		if err != nil {
			return err
		}
		tracked.Tags = []string{"go", "orm", "json"}
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	after, err := repo.Find(ctx, doc.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "orm", "json"}, after.Tags)
}
