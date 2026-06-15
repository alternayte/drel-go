package dreltest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/dreltest"
)

func TestNewSQLite_WithSchemaAndSeed(t *testing.T) {
	engine := dreltest.NewSQLite(t,
		dreltest.WithSchema("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"),
		dreltest.WithSeed(func(e *drel.Engine) error {
			_, err := e.Exec(context.Background(), "INSERT INTO users (name) VALUES ('alice'), ('bob')")
			return err
		}),
	)

	row := engine.QueryRow(context.Background(), "SELECT COUNT(*) FROM users")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != 2 {
		t.Fatalf("seeded rows = %d, want 2", count)
	}
}

// TestNewSQLite_SeedError proves a failing seed fails the test loudly rather
// than silently dropping the error. It runs the helper under a fake T that
// records Fatalf instead of aborting.
func TestNewSQLite_SeedError(t *testing.T) {
	ft := &fakeT{T: t}
	func() {
		defer func() { _ = recover() }() // fakeT.FailNow panics to unwind
		dreltest.NewSQLite(ft,
			dreltest.WithSeed(func(e *drel.Engine) error {
				return errors.New("boom")
			}),
		)
	}()
	if !ft.failed {
		t.Fatal("expected seed failure to fail the test, but it did not")
	}
}
