package drel_test

import (
	"context"
	"fmt"
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

	// Two rows with a duplicate name -> the INSERT fails atomically in a single batch.
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

// TestBulkInsert_MultiBatch_ErrorRollsBack_ReturnsZero proves the count-on-rollback
// fix for the genuine multi-batch scenario: batch 1 succeeds and increments the
// running total, then batch 2 fails, the whole transaction rolls back, and the
// return value must be 0 — not the partial count accumulated before the failure.
//
// ProductMeta has 3 insert columns, so safeBatchSize(3) = 1000. We insert 1001
// rows where the first 1000 are unique (batch 1 succeeds) and row 1001 duplicates
// row 0 (batch 2 fails). Without the fix, BulkInsert would return 1000; with the
// fix it returns 0.
func TestBulkInsert_MultiBatch_ErrorRollsBack_ReturnsZero(t *testing.T) {
	ctx := context.Background()
	eng, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

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

	// 1001 rows: indices 0..999 have unique names (they fill the first batch of
	// 1000 and succeed), then row 1000 duplicates row 0 (second batch fails).
	const firstBatch = 1000
	products := make([]*testmodels.Product, firstBatch+1)
	for i := 0; i < firstBatch; i++ {
		products[i] = &testmodels.Product{Name: fmt.Sprintf("product-%04d", i), Price: i + 1}
	}
	products[firstBatch] = &testmodels.Product{Name: "product-0000", Price: 9999} // collision

	n, err := repo.BulkInsert(ctx, products)
	if err == nil {
		t.Fatal("expected a unique-violation error on the second batch, got nil")
	}
	// The fix: return 0, not the partial 1000 accumulated before the failure.
	if n != 0 {
		t.Fatalf("expected 0 on rollback (got %d): partial count leaked despite full-tx rollback", n)
	}
	// Nothing must be persisted — the transaction rolled back entirely.
	cnt, err := repo.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("expected 0 rows persisted after rollback, got %d", cnt)
	}
}
