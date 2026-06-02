//go:build integration

package drel_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedManyProducts(t *testing.T, engine *drel.Engine, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 1; i <= n; i++ {
		_, err := engine.Exec(ctx,
			"INSERT INTO products (name, price, in_stock) VALUES ($1, $2, $3)",
			fmt.Sprintf("p-%03d", i), (i%4)*100, true)
		require.NoError(t, err)
	}
}

func TestIntegration_PageOffset(t *testing.T) {
	engine := setupTestDB(t)
	seedManyProducts(t, engine, 25)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	page, err := repo.OrderBy(testmodels.Products.ID.Asc()).Skip(20).Take(10).PageOffset(ctx)
	require.NoError(t, err)
	assert.Equal(t, 25, page.Total)
	assert.Equal(t, 3, page.Page)
	assert.Equal(t, 3, page.TotalPages)
	assert.Len(t, page.Items, 5)
	assert.False(t, page.HasMore)
}

func TestIntegration_CursorPage_KeysetWithTiebreaker(t *testing.T) {
	engine := setupTestDB(t)
	seedManyProducts(t, engine, 25)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	// Order by a non-unique column (price) so the PK tiebreaker is exercised
	// against real Postgres, where the multi-clause keyset OR/AND with $N
	// placeholders and int binding must round-trip correctly.
	seen := make([]int, 0, 25)
	cursor := ""
	pages := 0
	for {
		q := repo.OrderBy(testmodels.Products.Price.Asc()).Take(6)
		if cursor != "" {
			q = q.After(cursor)
		}
		page, err := q.Page(ctx)
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, pages, 10)
		for _, p := range page.Items {
			seen = append(seen, p.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}

	require.Len(t, seen, 25)
	dedup := map[int]bool{}
	for _, id := range seen {
		require.False(t, dedup[id], "row %d returned twice across pages", id)
		dedup[id] = true
	}
}
