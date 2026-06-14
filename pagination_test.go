package drel_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Pagination test model ───────────────────────────────────────────────────

type pageRow struct {
	ID   int
	Name string
	Rank int
}

var pageRowMeta = drel.ModelMeta[pageRow]{
	Table:    "page_rows",
	Columns:  []string{"id", "name", "rank"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*pageRow, error) {
		p := &pageRow{}
		err := row.Scan(&p.ID, &p.Name, &p.Rank)
		return p, err
	},
	PKValue: func(p *pageRow) any { return p.ID },
	ColumnValue: func(p *pageRow, idx int) any {
		switch idx {
		case 0:
			return p.ID
		case 1:
			return p.Name
		case 2:
			return p.Rank
		}
		return nil
	},
	InsertColumns: func(p *pageRow) ([]string, []any) {
		return []string{"name", "rank"}, []any{p.Name, p.Rank}
	},
}

func setupPageRows(t *testing.T, n int) (*drel.Engine, *drel.Repository[pageRow]) {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `CREATE TABLE page_rows (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		rank INTEGER NOT NULL
	)`)
	require.NoError(t, err)

	for i := 1; i <= n; i++ {
		// rank deliberately repeats so keyset pagination must rely on the PK tiebreaker.
		_, err = engine.Exec(ctx, `INSERT INTO page_rows (name, rank) VALUES (?, ?)`,
			fmt.Sprintf("row-%02d", i), i%3)
		require.NoError(t, err)
	}
	return engine, drel.NewRepository(engine, pageRowMeta)
}

// ─── Offset pagination ───────────────────────────────────────────────────────

func TestPageOffset_Math(t *testing.T) {
	engine, repo := setupPageRows(t, 25)
	_ = engine
	ctx := context.Background()

	page, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Skip(20).Take(10).PageOffset(ctx)
	require.NoError(t, err)
	assert.Equal(t, 25, page.Total)
	assert.Equal(t, 10, page.PageSize)
	assert.Equal(t, 3, page.Page)       // offset 20 / size 10 + 1
	assert.Equal(t, 3, page.TotalPages) // ceil(25/10)
	assert.Len(t, page.Items, 5)        // only 5 rows left after skipping 20
	assert.False(t, page.HasMore)       // 20 + 5 == 25
	assert.Equal(t, 21, page.Items[0].ID)
}

func TestPageOffset_FirstPageHasMore(t *testing.T) {
	_, repo := setupPageRows(t, 25)
	ctx := context.Background()

	page, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(10).PageOffset(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Page)
	assert.Len(t, page.Items, 10)
	assert.True(t, page.HasMore)
}

func TestPageOffset_RequiresLimit(t *testing.T) {
	_, repo := setupPageRows(t, 3)
	ctx := context.Background()
	_, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).PageOffset(ctx)
	assert.ErrorIs(t, err, drel.ErrPaginationNeedsLimit)
}

// ─── Cursor pagination ───────────────────────────────────────────────────────

func TestCursorPage_WalksAllRowsAscending(t *testing.T) {
	_, repo := setupPageRows(t, 25)
	ctx := context.Background()

	seen := make([]int, 0, 25)
	cursor := ""
	pages := 0
	for {
		q := repo.OrderBy(drel.NewOrderedCol[int]("rank").Asc()).Take(7)
		if cursor != "" {
			q = q.After(cursor)
		}
		page, err := q.Page(ctx)
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, pages, 10, "too many pages — cursor not advancing")
		for _, it := range page.Items {
			seen = append(seen, it.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		require.NotEmpty(t, cursor)
	}

	// Every row exactly once, no duplicates, no skips.
	require.Len(t, seen, 25)
	dedup := make(map[int]bool)
	for _, id := range seen {
		require.False(t, dedup[id], "row %d returned twice", id)
		dedup[id] = true
	}
}

func TestCursorPage_Descending(t *testing.T) {
	_, repo := setupPageRows(t, 12)
	ctx := context.Background()

	seen := make([]int, 0, 12)
	cursor := ""
	for {
		q := repo.OrderBy(drel.NewOrderedCol[int]("id").Desc()).Take(5)
		if cursor != "" {
			q = q.After(cursor)
		}
		page, err := q.Page(ctx)
		require.NoError(t, err)
		for _, it := range page.Items {
			seen = append(seen, it.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	require.Len(t, seen, 12)
	// Descending: first id is 12, last is 1, strictly decreasing.
	for i := 1; i < len(seen); i++ {
		assert.Greater(t, seen[i-1], seen[i])
	}
	assert.Equal(t, 12, seen[0])
	assert.Equal(t, 1, seen[len(seen)-1])
}

func TestCursorPage_RequiresOrderBy(t *testing.T) {
	_, repo := setupPageRows(t, 3)
	ctx := context.Background()
	_, err := repo.Take(2).Page(ctx)
	assert.ErrorIs(t, err, drel.ErrCursorPaginationNeedsOrderBy)
}

// catKind is a named string type used to exercise cursor encoding of a
// non-builtin (enum-like / uuid-like) order-key type.
type catKind string

type catRow struct {
	ID   int
	Kind catKind
}

var catRowMeta = drel.ModelMeta[catRow]{
	Table:    "cat_rows",
	Columns:  []string{"id", "kind"},
	PKColumn: "id",
	Scan: func(r drel.Row) (*catRow, error) {
		c := &catRow{}
		return c, r.Scan(&c.ID, &c.Kind)
	},
	PKValue: func(c *catRow) any { return c.ID },
	ColumnValue: func(c *catRow, i int) any {
		if i == 0 {
			return c.ID
		}
		return c.Kind
	},
}

func TestCursorPage_NamedTypeOrderKey(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()
	ctx := context.Background()
	_, err = engine.Exec(ctx, `CREATE TABLE cat_rows (id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL)`)
	require.NoError(t, err)
	for i := 1; i <= 9; i++ {
		_, err = engine.Exec(ctx, `INSERT INTO cat_rows (kind) VALUES (?)`,
			[]string{"a", "b", "c"}[i%3])
		require.NoError(t, err)
	}
	repo := drel.NewRepository(engine, catRowMeta)

	// Ordering by the named-type column must encode/decode cursors without error.
	seen := 0
	cursor := ""
	for {
		q := repo.OrderBy(drel.NewOrderedCol[catKind]("kind").Asc()).Take(4)
		if cursor != "" {
			q = q.After(cursor)
		}
		page, err := q.Page(ctx)
		require.NoError(t, err) // would error if gob couldn't encode catKind
		seen += len(page.Items)
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		require.NotEmpty(t, cursor)
	}
	assert.Equal(t, 9, seen)
}

func TestCursorPage_InvalidCursor(t *testing.T) {
	_, repo := setupPageRows(t, 3)
	ctx := context.Background()
	_, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(2).After("not-a-valid-cursor!!!").Page(ctx)
	assert.ErrorIs(t, err, drel.ErrInvalidCursor)
}

func TestPageOffset_RejectsZeroAndNegativeTake(t *testing.T) {
	_, repo := setupPageRows(t, 5)
	ctx := context.Background()

	_, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(0).PageOffset(ctx)
	assert.ErrorIs(t, err, drel.ErrInvalidPageSize)

	_, err = repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(-3).PageOffset(ctx)
	assert.ErrorIs(t, err, drel.ErrInvalidPageSize)
}

func TestCursorPage_RejectsZeroAndNegativeTake(t *testing.T) {
	_, repo := setupPageRows(t, 5)
	ctx := context.Background()

	_, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(0).Page(ctx)
	assert.ErrorIs(t, err, drel.ErrInvalidPageSize)

	_, err = repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(-1).Page(ctx)
	assert.ErrorIs(t, err, drel.ErrInvalidPageSize)
}

func TestCursorPage_IgnoresSkipOnFirstPage(t *testing.T) {
	_, repo := setupPageRows(t, 25)
	ctx := context.Background()

	// Skip(5) must be ignored by keyset Page; the first page must start at id 1.
	page, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Skip(5).Take(3).Page(ctx)
	require.NoError(t, err)
	require.Len(t, page.Items, 3)
	assert.Equal(t, 1, page.Items[0].ID)
	assert.Equal(t, 2, page.Items[1].ID)
	assert.Equal(t, 3, page.Items[2].ID)
}

func TestCursorPage_IgnoresSkipAcrossWalk(t *testing.T) {
	_, repo := setupPageRows(t, 25)
	ctx := context.Background()

	// A Skip set on the builder must not drop rows across the whole cursor walk.
	seen := make([]int, 0, 25)
	cursor := ""
	for {
		q := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Skip(5).Take(7)
		if cursor != "" {
			q = q.After(cursor)
		}
		page, err := q.Page(ctx)
		require.NoError(t, err)
		for _, it := range page.Items {
			seen = append(seen, it.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	require.Len(t, seen, 25)
	assert.Equal(t, 1, seen[0])
	assert.Equal(t, 25, seen[len(seen)-1])
}
