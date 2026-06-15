package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
)

func TestBulkInsert_ErrorRollsBack_ReturnsZero(t *testing.T) {
	ctx := context.Background()
	eng, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Unique name; the second batch will collide and fail mid-transaction.
	if _, err := eng.Exec(ctx, `CREATE TABLE products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		price INTEGER NOT NULL,
		in_stock BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	repo := drel.NewRepository(eng, testmodels.ProductMeta)

	// Two rows with a duplicate name -> the INSERT fails atomically.
	products := []*testmodels.Product{
		{Name: "dup", Price: 1},
		{Name: "dup", Price: 2},
	}
	n, err := repo.BulkInsert(ctx, products)
	if err == nil {
		t.Fatal("expected a unique-violation error, got nil")
	}
	if n != 0 {
		t.Fatalf("expected 0 on rollback, got %d", n)
	}
	// The whole transaction rolled back: nothing persisted.
	cnt, err := repo.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("expected 0 rows persisted after rollback, got %d", cnt)
	}
}
