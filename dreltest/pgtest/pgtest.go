package pgtest

import (
	"context"
	"time"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/driver/pgxdriver"
	"github.com/alternayte/drel/internal/migrate"
	"github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type config struct {
	migrationsDir string
	seedFn        func(*drel.Engine) error
}

// Option configures NewPostgres.
type Option func(*config)

// WithMigrations runs all pending migrations from dir after the container starts.
func WithMigrations(dir string) Option {
	return func(c *config) { c.migrationsDir = dir }
}

// WithSeed runs a seed function against the engine after the container starts
// (and after any migrations if WithMigrations is also provided). A non-nil
// error fails the test.
func WithSeed(fn func(*drel.Engine) error) Option {
	return func(c *config) { c.seedFn = fn }
}

// testingTB is the subset of *testing.T NewPostgres uses; it lets the
// seed-failure path be asserted with a fake T in tests. *testing.T satisfies it.
type testingTB interface {
	Helper()
	Cleanup(func())
	Fatalf(format string, args ...any)
}

// NewPostgres starts a Postgres container and returns a connected drel Engine.
// The container is torn down when the test completes via t.Cleanup.
func NewPostgres(t testingTB, opts ...Option) *drel.Engine {
	t.Helper()
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	ctx := context.Background()

	container, err := pgmodule.Run(ctx,
		"postgres:16-alpine",
		pgmodule.WithDatabase("dreltest"),
		pgmodule.WithUsername("dreltest"),
		pgmodule.WithPassword("dreltest"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("pgtest.NewPostgres: start container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("pgtest.NewPostgres: connection string: %v", err)
	}

	engine, err := drel.NewEngine(dsn)
	if err != nil {
		t.Fatalf("pgtest.NewPostgres: connect: %v", err)
	}
	t.Cleanup(func() { engine.Close() })

	if cfg.migrationsDir != "" {
		// Create a driver directly for the migration runner.
		drv, err := pgxdriver.New(ctx, dsn)
		if err != nil {
			t.Fatalf("pgtest.NewPostgres: migrate driver: %v", err)
		}
		defer drv.Close()

		runner := migrate.NewRunner(drv, cfg.migrationsDir, "postgres")
		if _, err := runner.Up(ctx); err != nil {
			t.Fatalf("pgtest.NewPostgres: migrations: %v", err)
		}
	}

	if cfg.seedFn != nil {
		if err := cfg.seedFn(engine); err != nil {
			t.Fatalf("pgtest.NewPostgres: seed: %v", err)
		}
	}

	return engine
}
