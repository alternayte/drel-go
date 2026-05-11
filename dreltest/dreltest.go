package dreltest

import (
	"context"
	"strings"
	"testing"

	"github.com/alternayte/drel"
)

// NewSQLite creates an in-memory SQLite engine for testing.
// The engine is automatically closed when the test completes.
func NewSQLite(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatalf("dreltest.NewSQLite: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	return engine
}

// Option configures NewSQLiteWith.
type Option func(*config)

type config struct {
	seedFn func(*drel.Engine)
}

// WithSeed runs a seed function after the database is created.
func WithSeed(fn func(*drel.Engine)) Option {
	return func(c *config) { c.seedFn = fn }
}

// Begin starts a savepoint-based transaction scope that auto-rolls back when
// the test completes. This provides per-test isolation without recreating the
// database schema.
//
// Uses SQLite SAVEPOINTs for isolation since SQLite only supports a single
// active transaction.
func Begin(t *testing.T, engine *drel.Engine) *drel.Engine {
	t.Helper()
	ctx := context.Background()

	// Sanitize test name for use as savepoint identifier.
	// Replace any non-alphanumeric chars with underscores.
	name := sanitizeName(t.Name())
	sp := "sp_" + name

	_, err := engine.Exec(ctx, "SAVEPOINT "+sp)
	if err != nil {
		t.Fatalf("dreltest.Begin: %v", err)
	}

	t.Cleanup(func() {
		_, _ = engine.Exec(ctx, "ROLLBACK TO SAVEPOINT "+sp)
		_, _ = engine.Exec(ctx, "RELEASE SAVEPOINT "+sp)
	})

	// Return the same engine — SQLite savepoints work on the same connection.
	return engine
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
