package dreltest_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/dreltest"
)

func TestNewSQLite(t *testing.T) {
	engine := dreltest.NewSQLite(t)
	ctx := context.Background()

	row := engine.QueryRow(ctx, "SELECT sqlite_version()")
	var version string
	if err := row.Scan(&version); err != nil {
		t.Fatalf("sqlite_version: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty sqlite version")
	}
}

func TestBegin_Isolation(t *testing.T) {
	engine := dreltest.NewSQLite(t)
	ctx := context.Background()

	_, err := engine.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("insert_in_savepoint", func(t *testing.T) {
		tx := dreltest.Begin(t, engine)
		_, err := tx.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "alice")
		if err != nil {
			t.Fatal(err)
		}

		// Visible within savepoint.
		row := tx.QueryRow(ctx, "SELECT COUNT(*) FROM test")
		var count int
		if err := row.Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("in-savepoint count: got %d, want 1", count)
		}
	})

	// After subtest cleanup, savepoint should have rolled back.
	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("after rollback: got %d rows, want 0", count)
	}
}

func TestBegin_MultipleSubtests(t *testing.T) {
	engine := dreltest.NewSQLite(t)
	ctx := context.Background()

	_, _ = engine.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")

	t.Run("first", func(t *testing.T) {
		tx := dreltest.Begin(t, engine)
		_, _ = tx.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "alice")
	})

	t.Run("second", func(t *testing.T) {
		tx := dreltest.Begin(t, engine)
		// Should be empty — first subtest's data was rolled back.
		row := tx.QueryRow(ctx, "SELECT COUNT(*) FROM test")
		var count int
		_ = row.Scan(&count)
		if count != 0 {
			t.Fatalf("expected clean state, got %d rows", count)
		}

		_, _ = tx.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "bob")
	})

	// Both subtests' data should be rolled back.
	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM test")
	var count int
	_ = row.Scan(&count)
	if count != 0 {
		t.Fatalf("expected clean state after all subtests, got %d rows", count)
	}
}
