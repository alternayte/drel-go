package drel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	queryHooks        []QueryHook

	logger        *slog.Logger
	tracer        Tracer
	devMode       bool
	slowThreshold time.Duration
	n1            *n1Detector
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
}

// detectDialect inspects the DSN and returns "sqlite" or "postgres".
// Patterns recognised as SQLite: "file:" prefix, "sqlite://" prefix,
// ":memory:", or a path ending with ".db".
// Everything else (including "postgres://" and "postgresql://") maps to "postgres".
func detectDialect(dsn string) string {
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
		case "sqlite":
			if drv == nil {
				d, err := sqlitedriver.New(dsn)
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
				d, err := pgxdriver.New(cfg.ctx, dsn)
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

	e := &Engine{
		drv: drv,
		dia: dia,
	}
	e.installObservability(cfg)
	return e, nil
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

// Close shuts down the underlying database connection.
func (e *Engine) Close() {
	e.drv.Close()
}

// Exec executes a raw SQL statement and returns the number of rows affected.
func (e *Engine) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return e.execInternal(ctx, sql, args...)
}

// Query executes a raw SQL query and returns the result rows.
// The caller must close the returned Rows when done.
func (e *Engine) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	return e.queryInternal(ctx, sql, args...)
}

// QueryRow executes a raw SQL query that is expected to return at most one row.
func (e *Engine) QueryRow(ctx context.Context, sql string, args ...any) Row {
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

func (e *Engine) execInternal(ctx context.Context, sql string, args ...any) (int64, error) {
	ctx, endSpan := e.startSpan(ctx, "drel.exec")
	start := time.Now()
	n, err := e.drv.Exec(ctx, sql, args...)
	endSpan(err)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return n, err
}

func (e *Engine) queryInternal(ctx context.Context, sql string, args ...any) (Rows, error) {
	ctx, endSpan := e.startSpan(ctx, "drel.query")
	start := time.Now()
	rows, err := e.drv.Query(ctx, sql, args...)
	endSpan(err)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return rows, err
}

func (e *Engine) queryRowInternal(ctx context.Context, sql string, args ...any) Row {
	ctx, endSpan := e.startSpan(ctx, "drel.queryRow")
	start := time.Now()
	row := e.drv.QueryRow(ctx, sql, args...)
	endSpan(nil)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), nil)
	return row
}

func (e *Engine) OnBeforeCommit(hook BeforeCommitHook) {
	e.beforeCommitHooks = append(e.beforeCommitHooks, hook)
}

func (e *Engine) OnAfterCommit(hook AfterCommitHook) {
	e.afterCommitHooks = append(e.afterCommitHooks, hook)
}
