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

func newSoftDeleteGuardEngine(t *testing.T) *drel.Engine {
	t.Helper()
	eng, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eng.Close() })
	ctx := context.Background()
	if _, err := eng.Exec(ctx, `CREATE TABLE sd_products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		price INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at DATETIME
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Exec(ctx, `INSERT INTO sd_products (name, price) VALUES ('A', 100), ('B', 200)`); err != nil {
		t.Fatal(err)
	}
	return eng
}

func TestBulkDelete_SoftDelete_NoWhere_ReturnsGuardError(t *testing.T) {
	eng := newSoftDeleteGuardEngine(t)
	repo := drel.NewRepository(eng, testmodels.SoftDeleteProductMeta)
	ctx := context.Background()

	// No user Where; the soft-delete auto-filter must NOT satisfy the guard.
	_, err := repo.OrderBy(testmodels.SoftDeleteProducts.Name.Asc()).
		BulkDelete(ctx)
	if !errors.Is(err, drel.ErrBulkDeleteRequiresFilter) {
		t.Fatalf("expected ErrBulkDeleteRequiresFilter for soft-delete model, got %v", err)
	}

	// Both rows must still be live (deleted_at IS NULL).
	live, err := repo.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live != 2 {
		t.Fatalf("expected 2 live rows (nothing soft-deleted), got %d", live)
	}
}

func TestBulkDelete_SoftDelete_AllRows_OptsOut(t *testing.T) {
	eng := newSoftDeleteGuardEngine(t)
	repo := drel.NewRepository(eng, testmodels.SoftDeleteProductMeta)
	ctx := context.Background()

	n, err := repo.AllRows().BulkDelete(ctx)
	if err != nil {
		t.Fatalf("AllRows should opt out of the guard, got %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows soft-deleted, got %d", n)
	}
}
