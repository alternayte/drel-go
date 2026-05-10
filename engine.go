package drel

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/dialect"
	"github.com/alternayte/drel/internal/dialect/postgres"
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/pgxdriver"
)

// Engine holds a database driver and dialect for executing queries.
type Engine struct {
	drv     driver.Driver
	dialect dialect.Dialect
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
		drv:     drv,
		dialect: postgres.New(),
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

// Driver returns the underlying database driver.
func (e *Engine) Driver() driver.Driver {
	return e.drv
}

// Dialect returns the SQL dialect used by this engine.
func (e *Engine) Dialect() dialect.Dialect {
	return e.dialect
}
