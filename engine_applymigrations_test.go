package drel_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alternayte/drel"
)

func TestEngine_ApplyMigrations_SQLite(t *testing.T) {
	dir := t.TempDir()
	up := filepath.Join(dir, "20240101000000_create_widgets.up.sql")
	down := filepath.Join(dir, "20240101000000_create_widgets.down.sql")
	if err := os.WriteFile(up, []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(down, []byte("DROP TABLE widgets;"), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(e.Close)
	ctx := context.Background()

	n, err := e.ApplyMigrations(ctx, dir)
	if err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}
	if n != 1 {
		t.Fatalf("applied = %d, want 1", n)
	}

	// The table must exist in the SAME in-memory database the engine uses.
	if _, err := e.Exec(ctx, "INSERT INTO widgets (name) VALUES ('a')"); err != nil {
		t.Fatalf("insert into migrated table: %v", err)
	}

	// Re-running is idempotent (already-applied migration is skipped).
	n2, err := e.ApplyMigrations(ctx, dir)
	if err != nil {
		t.Fatalf("ApplyMigrations (second): %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second apply = %d, want 0", n2)
	}
}
