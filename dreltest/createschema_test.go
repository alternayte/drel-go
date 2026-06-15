package dreltest_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/dreltest"
)

func TestCreateSchema(t *testing.T) {
	engine := dreltest.NewSQLite(t)
	dreltest.CreateSchema(t, engine,
		"CREATE TABLE a (id INTEGER PRIMARY KEY)",
		"CREATE TABLE b (id INTEGER PRIMARY KEY, a_id INTEGER REFERENCES a(id))",
	)

	ctx := context.Background()
	if _, err := engine.Exec(ctx, "INSERT INTO a (id) VALUES (1)"); err != nil {
		t.Fatalf("insert a: %v", err)
	}
	if _, err := engine.Exec(ctx, "INSERT INTO b (id, a_id) VALUES (1, 1)"); err != nil {
		t.Fatalf("insert b: %v", err)
	}
}

func TestCreateSchema_BadDDLFails(t *testing.T) {
	engine := dreltest.NewSQLite(t)
	ft := &fakeT{T: t}
	func() {
		defer func() { _ = recover() }()
		dreltest.CreateSchema(ft, engine, "CREATE NONSENSE")
	}()
	if !ft.failed {
		t.Fatal("expected bad DDL to fail the test, but it did not")
	}
}
