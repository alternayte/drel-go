package pgtest_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/dreltest/pgtest"
)

func TestNewPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires Docker")
	}

	engine := pgtest.NewPostgres(t)
	ctx := context.Background()

	row := engine.QueryRow(ctx, "SELECT 1")
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if n != 1 {
		t.Fatalf("got %d, want 1", n)
	}
}

func TestNewPostgres_WithSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires Docker")
	}

	engine := pgtest.NewPostgres(t, pgtest.WithSeed(func(e *drel.Engine) {
		ctx := context.Background()
		e.Exec(ctx, "CREATE TABLE test (id SERIAL PRIMARY KEY, name TEXT)")
		e.Exec(ctx, "INSERT INTO test (name) VALUES ($1)", "seeded")
	}))

	ctx := context.Background()
	row := engine.QueryRow(ctx, "SELECT name FROM test WHERE id = 1")
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "seeded" {
		t.Fatalf("got %q, want %q", name, "seeded")
	}
}
