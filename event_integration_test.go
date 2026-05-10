//go:build integration

package drel_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type UserCreated struct{ Name string }
type UserEmailChanged struct{ OldEmail, NewEmail string }

func setupEventTestDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Driver().Exec(ctx, `
		CREATE TABLE event_users (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			email      TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)
	return engine
}

func TestIntegration_AfterCommit_DispatchesEvents(t *testing.T) {
	engine := setupEventTestDB(t)
	ctx := context.Background()

	var received []any
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		received = append(received, events...)
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return nil
	})
	require.NoError(t, err)

	require.Len(t, received, 1)
	assert.Equal(t, UserCreated{Name: "Alice"}, received[0])
}

func TestIntegration_AfterCommit_NotCalledOnRollback(t *testing.T) {
	engine := setupEventTestDB(t)
	ctx := context.Background()

	var received []any
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		received = append(received, events...)
	})

	_ = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return fmt.Errorf("intentional rollback")
	})

	assert.Empty(t, received)
}

func TestIntegration_AfterCommit_MultipleHooksInOrder(t *testing.T) {
	engine := setupEventTestDB(t)
	ctx := context.Background()

	var order []int
	engine.OnAfterCommit(func(ctx context.Context, events []any) { order = append(order, 1) })
	engine.OnAfterCommit(func(ctx context.Context, events []any) { order = append(order, 2) })

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, []int{1, 2}, order)
}

func TestIntegration_AfterCommit_EventOrdering(t *testing.T) {
	engine := setupEventTestDB(t)
	ctx := context.Background()

	var received []any
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		received = append(received, events...)
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		user.RecordEvent(UserEmailChanged{OldEmail: "", NewEmail: "alice@example.com"})
		repo.Add(user)
		return nil
	})
	require.NoError(t, err)

	require.Len(t, received, 2)
	assert.IsType(t, UserCreated{}, received[0])
	assert.IsType(t, UserEmailChanged{}, received[1])
}

func TestIntegration_AfterCommit_MultipleEntities(t *testing.T) {
	engine := setupEventTestDB(t)
	ctx := context.Background()

	var received []any
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		received = append(received, events...)
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		u1 := testmodels.NewEventUser("Alice", "alice@example.com")
		u1.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(u1)
		u2 := testmodels.NewEventUser("Bob", "bob@example.com")
		u2.RecordEvent(UserCreated{Name: "Bob"})
		repo.Add(u2)
		return nil
	})
	require.NoError(t, err)

	assert.Len(t, received, 2)
}

func TestIntegration_AfterCommit_MidTxSaveChanges(t *testing.T) {
	engine := setupEventTestDB(t)
	ctx := context.Background()

	var received []any
	var dispatchedDuringSaveChanges bool

	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		received = append(received, events...)
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)

		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}

		if len(received) > 0 {
			dispatchedDuringSaveChanges = true
		}

		return nil
	})
	require.NoError(t, err)

	assert.False(t, dispatchedDuringSaveChanges)
	require.Len(t, received, 1)
	assert.Equal(t, UserCreated{Name: "Alice"}, received[0])
}
