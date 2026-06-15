package drel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alternayte/drel/internal/dberr"
	"github.com/alternayte/drel/internal/dialect"
	"github.com/alternayte/drel/internal/dialect/postgres"
	dialectsqlite "github.com/alternayte/drel/internal/dialect/sqlite"
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/pgxdriver"
	"github.com/alternayte/drel/internal/driver/sqlitedriver"
)

// Engine holds a database driver and dialect for executing queries.
type Engine struct {
	drv               driver.Driver
	dia               dialect.Dialect
	beforeCommitHooks []BeforeCommitHook
	afterCommitHooks  []AfterCommitHook
	eventSinks        []func(ctx context.Context, tx *Tx, events []any) error
	queryHooks        []QueryHook

	logger        *slog.Logger
	tracer        Tracer
	devMode       bool
	slowThreshold time.Duration
	queryTimeout  time.Duration
	n1            *n1Detector

	replicas  []driver.Driver
	rrCounter uint64
}

// Option configures Engine creation.
type Option func(*engineConfig)

type engineConfig struct {
	ctx context.Context
	drv driver.Driver
	dia dialect.Dialect

	logger        *slog.Logger
	queryLog      bool
	slowThreshold time.Duration
	tracer        Tracer
	devMode       bool
	queryTimeout  time.Duration

	replicaDSNs []string
	replicaDrvs []driver.Driver

	authToken  string
	poolConfig driver.PoolConfig
}

// detectDialect inspects the DSN and returns "sqlite" or "postgres".
// Patterns recognised as SQLite: "file:" prefix, "sqlite://" prefix,
// ":memory:", or a path ending with ".db".
// Everything else (including "postgres://" and "postgresql://") maps to "postgres".
func detectDialect(dsn string) string {
	if strings.HasPrefix(dsn, "libsql://") ||
		strings.HasPrefix(dsn, "wss://") ||
		strings.HasPrefix(dsn, "ws://") ||
		strings.HasPrefix(dsn, "http://") ||
		strings.HasPrefix(dsn, "https://") {
		// libSQL/Turso DSNs. http(s) covers a local sqld and Turso's HTTP
		// endpoint; a SQL database is never addressed over http otherwise.
		return "libsql"
	}
	if strings.HasPrefix(dsn, "file:") ||
		strings.HasPrefix(dsn, "sqlite://") ||
		dsn == ":memory:" ||
		strings.HasSuffix(dsn, ".db") {
		return "sqlite"
	}
	return "postgres"
}

// NewEngine creates a new Engine connected to the given DSN.
// The dialect and driver are auto-detected from the DSN unless overridden
// with WithDriver or WithDialect.
func NewEngine(dsn string, opts ...Option) (*Engine, error) {
	cfg := &engineConfig{
		ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	drv := cfg.drv
	dia := cfg.dia

	if drv == nil || dia == nil {
		detected := detectDialect(dsn)
		switch detected {
		case "libsql":
			// libSQL is SQLite-compatible: reuse the SQLite dialect.
			if drv == nil {
				d, err := newLibSQLDriver(applyAuthToken(dsn, cfg.authToken), cfg.poolConfig)
				if err != nil {
					return nil, fmt.Errorf("drel: open: %w", err)
				}
				drv = d
			}
			if dia == nil {
				dia = dialectsqlite.New()
			}
		case "sqlite":
			if drv == nil {
				d, err := sqlitedriver.New(dsn, cfg.poolConfig)
				if err != nil {
					return nil, fmt.Errorf("drel: open: %w", err)
				}
				drv = d
			}
			if dia == nil {
				dia = dialectsqlite.New()
			}
		default: // "postgres"
			if drv == nil {
				d, err := pgxdriver.New(cfg.ctx, dsn, cfg.poolConfig)
				if err != nil {
					return nil, fmt.Errorf("drel: open: %w", err)
				}
				drv = d
			}
			if dia == nil {
				dia = postgres.New()
			}
		}
	}

	// Open read replicas (same dialect as the primary). Reads round-robin across
	// them; writes and transactions always use the primary.
	var replicas []driver.Driver
	for _, rdsn := range cfg.replicaDSNs {
		rd, err := openDriverForDSN(cfg.ctx, rdsn, cfg.poolConfig)
		if err != nil {
			for _, r := range replicas {
				r.Close()
			}
			return nil, fmt.Errorf("drel: open read replica: %w", err)
		}
		replicas = append(replicas, rd)
	}
	replicas = append(replicas, cfg.replicaDrvs...)

	e := &Engine{
		drv:          drv,
		dia:          dia,
		replicas:     replicas,
		queryTimeout: cfg.queryTimeout,
	}
	e.installObservability(cfg)
	return e, nil
}

// openDriverForDSN opens a driver for a DSN using the same dialect detection as
// the primary connection.
func openDriverForDSN(ctx context.Context, dsn string, pc driver.PoolConfig) (driver.Driver, error) {
	switch detectDialect(dsn) {
	case "libsql":
		return newLibSQLDriver(dsn, pc)
	case "sqlite":
		return sqlitedriver.New(dsn, pc)
	default:
		return pgxdriver.New(ctx, dsn, pc)
	}
}

// WithMaxConns sets the maximum number of open database connections in the pool.
func WithMaxConns(n int) Option {
	return func(cfg *engineConfig) { cfg.poolConfig.MaxConns = n }
}

// WithConnMaxLifetime sets the maximum lifetime of a pooled connection before it
// is recycled (useful behind connection poolers / for failover).
func WithConnMaxLifetime(d time.Duration) Option {
	return func(cfg *engineConfig) { cfg.poolConfig.ConnMaxLifetime = d }
}

// WithConnMaxIdleTime sets how long a connection may sit idle before being closed.
func WithConnMaxIdleTime(d time.Duration) Option {
	return func(cfg *engineConfig) { cfg.poolConfig.ConnMaxIdleTime = d }
}

// WithSimpleProtocol makes the Postgres driver use pgx's simple query protocol
// (no server-side prepared statements), required for PgBouncer transaction or
// statement pooling. It is a no-op for SQLite and libSQL/Turso. The equivalent
// DSN escape hatch is "?default_query_exec_mode=simple_protocol".
func WithSimpleProtocol() Option {
	return func(cfg *engineConfig) { cfg.poolConfig.SimpleProtocol = true }
}

// WithQueryTimeout sets a default deadline applied to every engine-level query
// and exec that does not already carry a shorter deadline. Zero (the default)
// means no default timeout. It composes with caller-supplied deadlines — the
// shorter one wins. Per-builder .Timeout(d) overrides this default for that
// query. The default is NOT auto-applied to queries issued inside an explicit
// Tx, because a timeout firing mid-transaction aborts the whole transaction;
// use .Timeout(d) explicitly there if you accept that semantics.
func WithQueryTimeout(d time.Duration) Option {
	return func(cfg *engineConfig) { cfg.queryTimeout = d }
}

// WithAuthToken sets the authentication token for a libSQL/Turso connection.
// It is appended to the DSN as the authToken query parameter.
func WithAuthToken(token string) Option {
	return func(cfg *engineConfig) { cfg.authToken = token }
}

// applyAuthToken appends an authToken query parameter to a libSQL DSN if a token
// is provided and the DSN does not already carry one.
func applyAuthToken(dsn, token string) string {
	if token == "" || strings.Contains(dsn, "authToken=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "authToken=" + token
}

// WithReadReplica registers a read replica. Read queries issued through
// repositories (not within a transaction, and not forced to Primary) are routed
// round-robin across all registered replicas; writes and transactions always go
// to the primary.
func WithReadReplica(dsn string) Option {
	return func(cfg *engineConfig) {
		cfg.replicaDSNs = append(cfg.replicaDSNs, dsn)
	}
}

// WithContext sets the context used during engine creation.
func WithContext(ctx context.Context) Option {
	return func(cfg *engineConfig) {
		cfg.ctx = ctx
	}
}

// WithDriver overrides the driver used by the engine.
// When set, auto-detection is skipped for the driver.
func WithDriver(drv driver.Driver) Option {
	return func(cfg *engineConfig) {
		cfg.drv = drv
	}
}

// WithDialect overrides the SQL dialect used by the engine.
// When set, auto-detection is skipped for the dialect.
func WithDialect(dia dialect.Dialect) Option {
	return func(cfg *engineConfig) {
		cfg.dia = dia
	}
}

// Close shuts down the underlying database connection and any read replicas.
func (e *Engine) Close() {
	e.drv.Close()
	for _, r := range e.replicas {
		r.Close()
	}
}

// readDriver selects the driver for a read query: the primary when primary is
// true or no replicas are registered, otherwise a replica chosen round-robin.
func (e *Engine) readDriver(primary bool) driver.Driver {
	if primary || len(e.replicas) == 0 {
		return e.drv
	}
	n := atomic.AddUint64(&e.rrCounter, 1)
	return e.replicas[(n-1)%uint64(len(e.replicas))]
}

// Exec executes a raw SQL statement and returns the number of rows affected.
// $N placeholders are rewritten to ? on dialects that use ? (SQLite/libSQL).
func (e *Engine) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}
	return e.execInternal(ctx, sql, args...)
}

// Query executes a raw SQL query and returns the result rows.
// The caller must close the returned Rows when done. $N placeholders are
// rewritten to ? on dialects that use ? (SQLite/libSQL).
func (e *Engine) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}
	return e.queryInternal(ctx, sql, args...)
}

// QueryRow executes a raw SQL query that is expected to return at most one row.
// $N placeholders are rewritten to ? on dialects that use ? (SQLite/libSQL).
func (e *Engine) QueryRow(ctx context.Context, sql string, args ...any) Row {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}
	return e.queryRowInternal(ctx, sql, args...)
}

func (e *Engine) driver() driver.Driver {
	return e.drv
}

func (e *Engine) dialect() dialect.Dialect {
	return e.dia
}

// startSpan begins a tracing span for a query if a tracer is configured.
// It returns the (possibly augmented) context and a no-op-safe end function.
func (e *Engine) startSpan(ctx context.Context, name string) (context.Context, func(err error)) {
	if e.tracer == nil {
		return ctx, func(error) {}
	}
	spanCtx, span := e.tracer.Start(ctx, name)
	return spanCtx, func(err error) {
		span.RecordError(err)
		span.End()
	}
}

// withTimeout derives a child context bounded by the engine's default query
// timeout. It returns the original context (and a no-op cancel) when no default
// is configured or when the caller's existing deadline is already at or under
// the default. The returned cancel must always be deferred by the caller.
func (e *Engine) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return applyTimeout(ctx, e.queryTimeout)
}

// applyTimeout bounds ctx by d, returning ctx unchanged when d <= 0 or the
// existing caller deadline is already at or under d (shorter wins).
func applyTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= d {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

func (e *Engine) execInternal(ctx context.Context, sql string, args ...any) (int64, error) {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	ctx, endSpan := e.startSpan(ctx, "drel.exec")
	start := time.Now()
	n, err := e.drv.Exec(ctx, sql, args...)
	endSpan(err)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return n, dberr.Classify(err)
}

func (e *Engine) queryInternal(ctx context.Context, sql string, args ...any) (Rows, error) {
	return e.queryRouted(ctx, false, sql, args...)
}

// queryRouted executes a read query against a replica (primary=false) or the
// primary (primary=true), applying the engine default query timeout.
func (e *Engine) queryRouted(ctx context.Context, primary bool, sql string, args ...any) (Rows, error) {
	return e.queryRoutedTimeout(ctx, primary, e.queryTimeout, sql, args...)
}

// queryRoutedTimeout is queryRouted with an explicit timeout override (0 = use
// the engine default; a positive value overrides it for this call). The shorter
// of the override and any caller deadline wins.
func (e *Engine) queryRoutedTimeout(ctx context.Context, primary bool, timeout time.Duration, sql string, args ...any) (Rows, error) {
	if timeout <= 0 {
		timeout = e.queryTimeout
	}
	ctx, cancel := applyTimeout(ctx, timeout)
	defer cancel()
	ctx, endSpan := e.startSpan(ctx, "drel.query")
	start := time.Now()
	rows, err := e.readDriver(primary).Query(ctx, sql, args...)
	endSpan(err)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return rows, dberr.Classify(err)
}

func (e *Engine) queryRowInternal(ctx context.Context, sql string, args ...any) Row {
	return e.queryRowRouted(ctx, false, sql, args...)
}

func (e *Engine) queryRowRouted(ctx context.Context, primary bool, sql string, args ...any) Row {
	return e.queryRowRoutedTimeout(ctx, primary, e.queryTimeout, sql, args...)
}

func (e *Engine) queryRowRoutedTimeout(ctx context.Context, primary bool, timeout time.Duration, sql string, args ...any) Row {
	if timeout <= 0 {
		timeout = e.queryTimeout
	}
	// A QueryRow defers its error to Scan, so the derived context must outlive
	// this function; cancel is attached to the returned row's lifecycle via a
	// timeoutRow wrapper that cancels on Scan.
	rowCtx, cancel := applyTimeout(ctx, timeout)
	rowCtx, endSpan := e.startSpan(rowCtx, "drel.queryRow")
	start := time.Now()
	row := e.readDriver(primary).QueryRow(rowCtx, sql, args...)
	endSpan(nil)
	e.notifyQueryHooks(rowCtx, sql, args, time.Since(start), nil)
	return classifyRow{row: timeoutRow{row: row, cancel: cancel}}
}

func (e *Engine) OnBeforeCommit(hook BeforeCommitHook) {
	e.beforeCommitHooks = append(e.beforeCommitHooks, hook)
}

func (e *Engine) OnAfterCommit(hook AfterCommitHook) {
	e.afterCommitHooks = append(e.afterCommitHooks, hook)
}

// addEventSink registers a function that receives all committed events (including
// those from entities staged by before-commit hooks) after the hook-flush step.
// This is the registration point used by UseOutbox.
func (e *Engine) addEventSink(fn func(ctx context.Context, tx *Tx, events []any) error) {
	e.eventSinks = append(e.eventSinks, fn)
}

// timeoutRow defers a context cancel until Scan runs, so a row whose query used
// a derived (timeout) context does not leak the cancel func. The cancel is
// idempotent.
type timeoutRow struct {
	row    driver.Row
	cancel context.CancelFunc
}

func (r timeoutRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	r.cancel()
	return err
}
