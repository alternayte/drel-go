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

// TestObs_Tracer_TransactionPath verifies that executions inside an engine
// Transaction emit tracing spans via the tx-path instrumentation (tx.execInternal,
// tx.queryInternal, tx.queryRowInternal). The assertion is delta-based: we snapshot
// len(tr.started) after setup (which itself emits a drel.exec span for the
// CREATE TABLE) and then confirm that new spans were recorded only by the
// in-transaction work. Removing the startSpan calls from tx.go must cause this
// test to fail.
func TestObs_Tracer_TransactionPath(t *testing.T) {
	tr := &fakeTracer{}
	engine := newObsEngine(t, drel.WithTracer(tr))
	ctx := context.Background()

	// Snapshot the span count after setup; CREATE TABLE already emitted spans.
	tr.mu.Lock()
	baseline := len(tr.started)
	tr.mu.Unlock()

	// Use tx.Exec (raw exec inside the transaction) to exercise tx.execInternal
	// directly, guaranteeing a "drel.exec" span on the tx path regardless of
	// which insert strategy the dialect chooses.
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO items (title) VALUES ('hello')`)
		return err
	}))

	tr.mu.Lock()
	defer tr.mu.Unlock()
	newSpans := tr.started[baseline:]
	assert.NotEmpty(t, newSpans, "transaction path should emit at least one span")
	assert.Contains(t, newSpans, "drel.exec",
		"tx.Exec inside a transaction should emit a drel.exec span via tx-path instrumentation")
	assert.Equal(t, len(tr.started), tr.ended, "every started span should end")
}

// TestObs_Tracer_BulkInsert verifies that BulkInsert emits tracing spans.
// Like TestObs_Tracer_TransactionPath the assertion is delta-based: we
// record the span count right after setup so that the CREATE TABLE span
// already present does not satisfy the assertion for the bulk path.
// Removing the startSpan call from bulk.go must cause this test to fail.
func TestObs_Tracer_BulkInsert(t *testing.T) {
	tr := &fakeTracer{}
	engine := newObsEngine(t, drel.WithTracer(tr))
	repo := drel.NewRepository(engine, sqliteItemMeta)
	ctx := context.Background()

	// Snapshot the span count after setup.
	tr.mu.Lock()
	baseline := len(tr.started)
	tr.mu.Unlock()

	// BulkInsert exercises a separate code path from the tracked-mutation path.
	_, err := repo.BulkInsert(ctx, []*sqliteItem{{Title: "a"}, {Title: "b"}})
	require.NoError(t, err)

	tr.mu.Lock()
	defer tr.mu.Unlock()
	newSpans := tr.started[baseline:]
	assert.NotEmpty(t, newSpans, "BulkInsert should emit at least one span")
	assert.Contains(t, newSpans, "drel.exec",
		"BulkInsert should emit a drel.exec span for the batch statement")
	assert.Equal(t, len(tr.started), tr.ended, "every started span should end")
}

func TestObs_HookRegistration_Race(t *testing.T) {
	engine := newObsEngine(t)
	repo := drel.NewRepository(engine, sqliteItemMeta)
	ctx := context.Background()

	var wg sync.WaitGroup
	// Registrar goroutine: register hooks while queries run.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			engine.OnQuery(func(context.Context, drel.QueryEvent) {})
		}
	}()
	// Reader goroutines: run queries that range over the hook slice.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, _ = repo.All(ctx)
			}
		}()
	}
	wg.Wait()
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
