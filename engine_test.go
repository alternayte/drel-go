package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
)

func TestDetectDialect_Memory(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatalf("NewEngine(:memory:): %v", err)
	}
	defer engine.Close()

	row := engine.QueryRow(context.Background(), "SELECT sqlite_version()")
	var version string
	if err := row.Scan(&version); err != nil {
		t.Fatalf("sqlite_version scan: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty sqlite version")
	}
}

func TestDetectDialect_FilePrefix(t *testing.T) {
	t.TempDir() // ensure temp dir exists
	engine, err := drel.NewEngine("file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("NewEngine(file:...): %v", err)
	}
	defer engine.Close()

	row := engine.QueryRow(context.Background(), "SELECT sqlite_version()")
	var version string
	if err := row.Scan(&version); err != nil {
		t.Fatalf("sqlite_version scan: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty sqlite version for file: prefix")
	}
}

func TestDetectDialect_DotDB(t *testing.T) {
	dir := t.TempDir()
	dsn := dir + "/test.db"

	engine, err := drel.NewEngine(dsn)
	if err != nil {
		t.Fatalf("NewEngine(%s): %v", dsn, err)
	}
	defer engine.Close()

	row := engine.QueryRow(context.Background(), "SELECT sqlite_version()")
	var version string
	if err := row.Scan(&version); err != nil {
		t.Fatalf("sqlite_version scan: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty sqlite version for .db suffix")
	}
}
