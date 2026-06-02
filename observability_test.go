package drel_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), &buf
}

func newObsEngine(t *testing.T, opts ...drel.Option) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:", opts...)
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })
	_, err = engine.Exec(context.Background(),
		`CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL,
		 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	return engine
}

func TestObs_QueryLog(t *testing.T) {
	logger, buf := bufLogger()
	engine := newObsEngine(t, drel.WithLogger(logger), drel.WithQueryLog(true))
	repo := drel.NewRepository(engine, sqliteItemMeta)
	_, err := repo.All(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "drel query")
}

func TestObs_SlowQuery(t *testing.T) {
	logger, buf := bufLogger()
	engine := newObsEngine(t, drel.WithLogger(logger), drel.WithSlowQueryThreshold(time.Nanosecond))
	repo := drel.NewRepository(engine, sqliteItemMeta)
	_, err := repo.All(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "slow query")
}

func TestObs_DevMode_UnboundedQuery(t *testing.T) {
	logger, buf := bufLogger()
	engine := newObsEngine(t, drel.WithDevMode(), drel.WithLogger(logger))
	repo := drel.NewRepository(engine, sqliteItemMeta)

	_, err := repo.All(context.Background()) // SELECT without LIMIT → unbounded
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "unbounded query")

	buf.Reset()
	_, _ = repo.Limit(5).All(context.Background()) // bounded → no warning
	assert.NotContains(t, buf.String(), "unbounded query")
}

func TestObs_DevMode_UnusedTracking(t *testing.T) {
	logger, buf := bufLogger()
	engine := newObsEngine(t, drel.WithDevMode(), drel.WithLogger(logger))
	ctx := context.Background()

	// Seed a row.
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, sqliteItemMeta).Add(&sqliteItem{Title: "x"})
		return tx.SaveChanges(ctx)
	}))
	buf.Reset()

	// Load (tracked) but never modify.
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		_, err := drel.NewTxRepository(tx, sqliteItemMeta).All(ctx)
		return err
	}))
	assert.Contains(t, buf.String(), "never modified")
}

// fakeTracer records span lifecycle for assertions.
type fakeTracer struct {
	mu      sync.Mutex
	started []string
	ended   int
}

type fakeSpan struct{ tr *fakeTracer }

func (t *fakeTracer) Start(ctx context.Context, name string) (context.Context, drel.Span) {
	t.mu.Lock()
	t.started = append(t.started, name)
	t.mu.Unlock()
	return ctx, &fakeSpan{tr: t}
}
func (s *fakeSpan) End() {
	s.tr.mu.Lock()
	s.tr.ended++
	s.tr.mu.Unlock()
}
func (s *fakeSpan) RecordError(error) {}

func TestObs_Tracer(t *testing.T) {
	tr := &fakeTracer{}
	engine := newObsEngine(t, drel.WithTracer(tr))
	repo := drel.NewRepository(engine, sqliteItemMeta)
	_, err := repo.All(context.Background())
	require.NoError(t, err)

	tr.mu.Lock()
	defer tr.mu.Unlock()
	assert.NotEmpty(t, tr.started, "a span should have started")
	assert.Contains(t, tr.started, "drel.query")
	assert.Equal(t, len(tr.started), tr.ended, "every started span should end")
}

func TestObs_N1Detector(t *testing.T) {
	logger, buf := bufLogger()
	engine := newObsEngine(t, drel.WithDevMode(), drel.WithLogger(logger))
	repo := drel.NewRepository(engine, sqliteItemMeta)
	ctx := context.Background()

	// Repeat the same Find shape many times → N+1 heuristic fires.
	for i := 0; i < 12; i++ {
		_, _ = repo.Find(ctx, 1) // ErrNotFound is fine; the query shape repeats
	}
	assert.Contains(t, buf.String(), "N+1")
}
