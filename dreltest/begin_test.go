package dreltest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/dreltest"
)

// TestBegin_FileBackedRollback is the load-bearing test: with a file-backed
// SQLite engine (multi-connection pool), Begin must still roll back writes.
// The pre-fix raw-SAVEPOINT implementation could land SAVEPOINT and ROLLBACK on
// different pooled connections and silently fail to undo.
func TestBegin_FileBackedRollback(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "begin.db")
	engine, err := drel.NewEngine("file:" + dbPath)
	if err != nil {
		t.Fatalf("NewEngine(file): %v", err)
	}
	t.Cleanup(engine.Close)

	ctx := context.Background()
	if _, err := engine.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("write_in_tx", func(t *testing.T) {
		tx := dreltest.Begin(t, engine)
		if _, err := tx.Exec(ctx, "INSERT INTO t (name) VALUES ('x')"); err != nil {
			t.Fatalf("insert: %v", err)
		}
		row := tx.QueryRow(ctx, "SELECT COUNT(*) FROM t")
		var c int
		if err := row.Scan(&c); err != nil {
			t.Fatal(err)
		}
		if c != 1 {
			t.Fatalf("in-tx count = %d, want 1", c)
		}
	})

	// After the subtest's cleanup ran, the write must be rolled back.
	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM t")
	var c int
	if err := row.Scan(&c); err != nil {
		t.Fatal(err)
	}
	if c != 0 {
		t.Fatalf("after rollback count = %d, want 0 (file-backed isolation broken)", c)
	}
}

// TestBegin_RejectsPostgresDialect proves the dialect guard fires without a
// real Postgres: an engine forced to the Postgres dialect must make Begin fail.
func TestBegin_RejectsPostgresDialect(t *testing.T) {
	// A SQLite driver under the Postgres dialect — enough to exercise the guard,
	// which checks engine.DialectName() before touching the driver.
	engine, err := drel.NewEngine("file:"+filepath.Join(t.TempDir(), "g.db"),
		drel.WithDialect(pgDialect()))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(engine.Close)

	ft := &fakeT{T: t}
	func() {
		defer func() { _ = recover() }()
		dreltest.Begin(ft, engine)
	}()
	if !ft.failed {
		t.Fatal("expected Begin to fail on a non-sqlite engine, but it did not")
	}
}
