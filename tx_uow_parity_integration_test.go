//go:build integration

package drel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAuthorsPG creates a Postgres engine with the authors+books schema
// using the same helper as integration_include_test.go (setupRelationDB).
func setupAuthorsPG(t *testing.T) *drel.Engine {
	return setupRelationDB(t)
}

func authorCount(t *testing.T, engine *drel.Engine) int {
	t.Helper()
	repo := drel.NewRepository(engine, authorMeta)
	n, err := repo.Count(context.Background())
	require.NoError(t, err)
	return n
}

func TestIntegration_TxQuery_MultiRow(t *testing.T) {
	engine := setupAuthorsPG(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO authors (name) VALUES ($1)", "Ada"); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT name FROM authors ORDER BY id")
		if err != nil {
			return err
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			names = append(names, n)
		}
		require.NoError(t, rows.Err())
		assert.Contains(t, names, "Ada")
		return nil
	})
	require.NoError(t, err)
}

func TestIntegration_SelectAggregate_InsideTx(t *testing.T) {
	engine := setupAuthorsPG(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO authors (name) VALUES ($1), ($2)", "Grace", "Linus"); err != nil {
			return err
		}
		repo := drel.NewTxRepository(tx, authorMeta)
		n, err := drel.Aggregate[int](ctx, repo.AsNoTracking(), drel.CountCol(drel.ColRef("id")))
		if err != nil {
			return err
		}
		assert.GreaterOrEqual(t, n, 2, "aggregate inside tx must count uncommitted rows")
		return nil
	})
	require.NoError(t, err)
}

func TestIntegration_TxInclude_OnTxConnection(t *testing.T) {
	engine := setupAuthorsPG(t)
	ctx := context.Background()

	// Seed an author with a book committed.
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO authors (id, name) VALUES (1, 'Author1') ON CONFLICT (id) DO NOTHING"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO books (title, author_id) VALUES ('Committed', 1)")
		return err
	}))

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		// Add an uncommitted book inside this tx.
		if _, err := tx.Exec(ctx, "INSERT INTO books (title, author_id) VALUES ('InTx', 1)"); err != nil {
			return err
		}
		repo := drel.NewTxRepository(tx, authorMeta)
		a, err := repo.Include(drel.NewIncludeSpec(booksRelation)).Find(ctx, 1)
		if err != nil {
			return err
		}
		titles := make([]string, 0, len(a.Books))
		for _, b := range a.Books {
			titles = append(titles, b.Title)
		}
		assert.Contains(t, titles, "Committed")
		assert.Contains(t, titles, "InTx", "tx include must see the uncommitted book")
		return nil
	})
	require.NoError(t, err)
}

func TestIntegration_BulkInsert_InsideTx_RollsBack(t *testing.T) {
	engine := setupAuthorsPG(t)
	ctx := context.Background()

	before := authorCount(t, engine)
	sentinel := errors.New("boom")
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, authorMeta)
		if _, err := repo.BulkInsert(ctx, []*Author{{Name: "B1"}, {Name: "B2"}}); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, before, authorCount(t, engine), "bulk insert must roll back with the caller tx")
}

func TestIntegration_TxBatch_OnTxConnection(t *testing.T) {
	engine := setupAuthorsPG(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO authors (name) VALUES ($1)", "BatchedOnly"); err != nil {
			return err
		}
		repo := drel.NewRepository(engine, authorMeta)
		b := tx.NewBatch()
		cnt := drel.BatchCount(b, repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()))
		if err := b.Execute(ctx); err != nil {
			return err
		}
		n, err := cnt.Result()
		if err != nil {
			return err
		}
		assert.GreaterOrEqual(t, n, 1, "tx batch must see uncommitted insert")
		return nil
	})
	require.NoError(t, err)
}
