//go:build integration

package pgtest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/dreltest/pgtest"
)

// recordingT intercepts Fatalf so the seed-failure path of NewPostgres can be
// asserted without aborting the integration test.
type recordingT struct {
	*testing.T
	failed bool
}

func (r *recordingT) Fatalf(format string, args ...any) { r.failed = true; r.FailNow() }
func (r *recordingT) FailNow()                          { panic("recordingT.FailNow") }

func TestNewPostgres_SeedErrorFailsLoudly(t *testing.T) {
	rt := &recordingT{T: t}
	func() {
		defer func() { _ = recover() }()
		pgtest.NewPostgres(rt, pgtest.WithSeed(func(e *drel.Engine) error {
			return errors.New("seed boom")
		}))
	}()
	if !rt.failed {
		t.Fatal("expected NewPostgres to fail the test on a seed error")
	}
}

func TestNewPostgres_WithMigrationsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20240101000000_init.up.sql"),
		[]byte("CREATE TABLE accounts (id SERIAL PRIMARY KEY, name TEXT NOT NULL);"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20240101000000_init.down.sql"),
		[]byte("DROP TABLE accounts;"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := pgtest.NewPostgres(t,
		pgtest.WithMigrations(dir),
		pgtest.WithSeed(func(e *drel.Engine) error {
			_, err := e.Exec(context.Background(), "INSERT INTO accounts (name) VALUES ('acme')")
			return err
		}),
	)

	row := engine.QueryRow(context.Background(), "SELECT name FROM accounts")
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if name != "acme" {
		t.Fatalf("name = %q, want %q", name, "acme")
	}
}
