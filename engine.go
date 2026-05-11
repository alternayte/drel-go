package drel

import (
	"context"
	"fmt"
	"time"

	"github.com/alternayte/drel/internal/dialect"
	"github.com/alternayte/drel/internal/dialect/postgres"
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/pgxdriver"
)

// Engine holds a database driver and dialect for executing queries.
type Engine struct {
	drv               driver.Driver
	dia               dialect.Dialect
	beforeCommitHooks []BeforeCommitHook
	afterCommitHooks  []AfterCommitHook
	queryHooks        []QueryHook
}

// Option configures Engine creation.
type Option func(*engineConfig)

type engineConfig struct {
	ctx context.Context
}

// NewEngine creates a new Engine connected to the given DSN.
func NewEngine(dsn string, opts ...Option) (*Engine, error) {
	cfg := &engineConfig{
		ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	drv, err := pgxdriver.New(cfg.ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("drel: open: %w", err)
	}

	return &Engine{
		drv: drv,
		dia: postgres.New(),
	}, nil
}

// WithContext sets the context used during engine creation.
func WithContext(ctx context.Context) Option {
	return func(cfg *engineConfig) {
		cfg.ctx = ctx
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

func (e *Engine) execInternal(ctx context.Context, sql string, args ...any) (int64, error) {
	start := time.Now()
	n, err := e.drv.Exec(ctx, sql, args...)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return n, err
}

func (e *Engine) queryInternal(ctx context.Context, sql string, args ...any) (Rows, error) {
	start := time.Now()
	rows, err := e.drv.Query(ctx, sql, args...)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return rows, err
}

func (e *Engine) queryRowInternal(ctx context.Context, sql string, args ...any) Row {
	start := time.Now()
	row := e.drv.QueryRow(ctx, sql, args...)
	e.notifyQueryHooks(ctx, sql, args, time.Since(start), nil)
	return row
}

func (e *Engine) OnBeforeCommit(hook BeforeCommitHook) {
	e.beforeCommitHooks = append(e.beforeCommitHooks, hook)
}

func (e *Engine) OnAfterCommit(hook AfterCommitHook) {
	e.afterCommitHooks = append(e.afterCommitHooks, hook)
}
