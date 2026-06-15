package drel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
)

func newBulkGuardEngine(t *testing.T) *drel.Engine {
	t.Helper()
	eng, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eng.Close() })
	ctx := context.Background()
	if _, err := eng.Exec(ctx, `CREATE TABLE products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		price INTEGER NOT NULL,
		in_stock BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Exec(ctx, `INSERT INTO products (name, price) VALUES ('a', 1), ('b', 2)`); err != nil {
		t.Fatal(err)
	}
	return eng
}

func TestBulkUpdate_NoWhere_ReturnsGuardError(t *testing.T) {
	eng := newBulkGuardEngine(t)
	repo := drel.NewRepository(eng, testmodels.ProductMeta)
	ctx := context.Background()

	// Reached via OrderBy with no Where: must be guarded.
	_, err := repo.OrderBy(testmodels.Products.Price.Asc()).
		BulkUpdate(ctx, drel.Set(testmodels.Products.Price, 999))
	if !errors.Is(err, drel.ErrBulkUpdateRequiresFilter) {
		t.Fatalf("expected ErrBulkUpdateRequiresFilter, got %v", err)
	}

	// Rows must be untouched.
	cnt, err := repo.Where(testmodels.Products.Price.Eq(999)).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("expected 0 rows changed, got %d", cnt)
	}
}

func TestBulkUpdate_WithWhere_Succeeds(t *testing.T) {
	eng := newBulkGuardEngine(t)
	repo := drel.NewRepository(eng, testmodels.ProductMeta)
	ctx := context.Background()

	n, err := repo.Where(testmodels.Products.Name.Eq("a")).
		BulkUpdate(ctx, drel.Set(testmodels.Products.Price, 999))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row updated, got %d", n)
	}
}

func TestBulkUpdate_AllRows_OptsOutOfGuard(t *testing.T) {
	eng := newBulkGuardEngine(t)
	repo := drel.NewRepository(eng, testmodels.ProductMeta)
	ctx := context.Background()

	n, err := repo.OrderBy(testmodels.Products.Price.Asc()).
		AllRows().
		BulkUpdate(ctx, drel.Set(testmodels.Products.Price, 999))
	if err != nil {
		t.Fatalf("AllRows should opt out of the guard, got %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows updated, got %d", n)
	}
}
