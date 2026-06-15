package dreltest_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/dreltest"
)

func TestNewSQLite_WithMigrations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20240101000000_init.up.sql"),
		[]byte("CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT NOT NULL);"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20240101000000_init.down.sql"),
		[]byte("DROP TABLE items;"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := dreltest.NewSQLite(t,
		dreltest.WithMigrations(dir),
		dreltest.WithSeed(func(e *drel.Engine) error {
			_, err := e.Exec(context.Background(), "INSERT INTO items (label) VALUES ('a')")
			return err
		}),
	)

	row := engine.QueryRow(context.Background(), "SELECT label FROM items")
	var label string
	if err := row.Scan(&label); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if label != "a" {
		t.Fatalf("label = %q, want %q", label, "a")
	}
}
