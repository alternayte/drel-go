//go:build integration

package drel_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type UserCreated struct{ Name string }
type UserEmailChanged struct{ OldEmail, NewEmail string }

func setupEventTestDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `
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

// ---------------------------------------------------------------------------
// OnBeforeCommit tests — separate DB with hook_log table
// ---------------------------------------------------------------------------

func setupBeforeCommitTestDB(t *testing.T) *drel.Engine {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dreltest"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	engine, err := drel.NewEngine(connStr, drel.WithContext(ctx))
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	_, err = engine.Exec(ctx, `
		CREATE TABLE event_users (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			email      TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)

	_, err = engine.Exec(ctx, `
		CREATE TABLE hook_log (
			id         SERIAL PRIMARY KEY,
			event_type TEXT NOT NULL,
			payload    TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	return engine
}

func TestIntegration_BeforeCommit_ExecWritesWithinTx(t *testing.T) {
	engine := setupBeforeCommitTestDB(t)
	ctx := context.Background()

	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		for _, e := range events {
			_, err := tx.Exec(ctx,
				"INSERT INTO hook_log (event_type, payload) VALUES ($1, $2)",
				fmt.Sprintf("%T", e), fmt.Sprintf("%v", e))
			if err != nil {
				return err
			}
		}
		return nil
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return nil
	})
	require.NoError(t, err)

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM hook_log")
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 1, count)
}

func TestIntegration_BeforeCommit_ErrorRollsBack(t *testing.T) {
	engine := setupBeforeCommitTestDB(t)
	ctx := context.Background()

	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		return fmt.Errorf("hook error")
	})

	var afterCommitCalled bool
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		afterCommitCalled = true
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook error")

	assert.False(t, afterCommitCalled)

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM event_users")
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count)
}

func TestIntegration_BeforeCommit_RollbackUndoesHookWrites(t *testing.T) {
	engine := setupBeforeCommitTestDB(t)
	ctx := context.Background()

	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		_, err := tx.Exec(ctx, "INSERT INTO hook_log (event_type, payload) VALUES ($1, $2)", "test", "data")
		return err
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return fmt.Errorf("user-level rollback")
	})
	require.Error(t, err)

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM hook_log")
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count)
}

func TestIntegration_BeforeCommit_TxRepoAddsEntity(t *testing.T) {
	engine := setupBeforeCommitTestDB(t)
	ctx := context.Background()

	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		audit := testmodels.NewEventUser("audit-bot", "audit@system")
		repo.Add(audit)
		return nil
	})

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.EventUserMeta)
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		repo.Add(user)
		return nil
	})
	require.NoError(t, err)

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM event_users")
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 2, count)
}

func TestIntegration_BeforeCommit_HookAddedEntityEventToOutbox(t *testing.T) {
	engine := setupBeforeCommitTestDB(t)
	ctx := context.Background()

	_, err := engine.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS event_outbox (
			id      BIGSERIAL PRIMARY KEY,
			type    TEXT NOT NULL,
			payload JSONB NOT NULL
		)`)
	require.NoError(t, err)
	engine.UseOutbox("event_outbox")

	var received []any
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		received = append(received, events...)
	})
	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		audit := testmodels.NewEventUser("audit-bot", "audit@system")
		audit.RecordEvent(UserCreated{Name: "audit-bot"})
		drel.NewTxRepository(tx, testmodels.EventUserMeta).Add(audit)
		return nil
	})

	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		user := testmodels.NewEventUser("Alice", "alice@example.com")
		user.RecordEvent(UserCreated{Name: "Alice"})
		drel.NewTxRepository(tx, testmodels.EventUserMeta).Add(user)
		return nil
	})
	require.NoError(t, err)

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM event_outbox")
	var outboxCount int
	require.NoError(t, row.Scan(&outboxCount))
	assert.Equal(t, 2, outboxCount)

	assert.Contains(t, received, UserCreated{Name: "Alice"})
	assert.Contains(t, received, UserCreated{Name: "audit-bot"})
	assert.Len(t, received, 2)
}
