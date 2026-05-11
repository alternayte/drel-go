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

func setupAuditTestDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `
		CREATE TABLE a_products (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			price      INTEGER NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			updated_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)
	return engine
}

func TestIntegration_Audit_CreateWithActor(t *testing.T) {
	engine := setupAuditTestDB(t)
	ctx := context.Background()

	actorCtx := drel.WithActor(ctx, "user-123")

	err := engine.Transaction(actorCtx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.AuditProductMeta)
		p := testmodels.NewAuditProduct("Widget", 1000)
		repo.Add(p)
		return nil
	})
	require.NoError(t, err)

	// Read back via raw SQL to verify audit columns
	row := engine.QueryRow(ctx,
		"SELECT created_by, updated_by FROM a_products WHERE name = $1", "Widget")
	var createdBy, updatedBy string
	require.NoError(t, row.Scan(&createdBy, &updatedBy))
	assert.Equal(t, "user-123", createdBy)
	assert.Equal(t, "user-123", updatedBy)
}

func TestIntegration_Audit_UpdateWithActor(t *testing.T) {
	engine := setupAuditTestDB(t)
	ctx := context.Background()

	// Insert with actor A
	actorACtx := drel.WithActor(ctx, "actor-A")
	var productID int
	err := engine.Transaction(actorACtx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.AuditProductMeta)
		p := testmodels.NewAuditProduct("Gadget", 2500)
		repo.Add(p)
		if err := tx.SaveChanges(actorACtx); err != nil {
			return err
		}
		productID = p.ID()
		return nil
	})
	require.NoError(t, err)

	// Update with actor B
	actorBCtx := drel.WithActor(ctx, "actor-B")
	err = engine.Transaction(actorBCtx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.AuditProductMeta)
		p, err := repo.Find(actorBCtx, productID)
		if err != nil {
			return err
		}
		p.SetName("UpdatedGadget")
		return nil
	})
	require.NoError(t, err)

	// Verify created_by unchanged, updated_by changed
	row := engine.QueryRow(ctx,
		"SELECT created_by, updated_by FROM a_products WHERE id = $1", productID)
	var createdBy, updatedBy string
	require.NoError(t, row.Scan(&createdBy, &updatedBy))
	assert.Equal(t, "actor-A", createdBy, "created_by should remain as original actor")
	assert.Equal(t, "actor-B", updatedBy, "updated_by should reflect the updating actor")
}

func TestIntegration_Audit_NoActorInContext(t *testing.T) {
	engine := setupAuditTestDB(t)
	ctx := context.Background()

	// Insert without setting any actor in context
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.AuditProductMeta)
		p := testmodels.NewAuditProduct("NoActor", 500)
		repo.Add(p)
		return nil
	})
	require.NoError(t, err)

	// Verify created_by is empty string (not an error)
	row := engine.QueryRow(ctx,
		"SELECT created_by, updated_by FROM a_products WHERE name = $1", "NoActor")
	var createdBy, updatedBy string
	require.NoError(t, row.Scan(&createdBy, &updatedBy))
	assert.Equal(t, "", createdBy, "created_by should be empty when no actor set")
	assert.Equal(t, "", updatedBy, "updated_by should be empty when no actor set")
}
