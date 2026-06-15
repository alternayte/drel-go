package drel_test

import (
	"context"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test model for Select/Aggregate/GroupBy ────────────────────────────────

type selectProduct struct {
	ID        int
	Name      string
	Category  string
	Price     float64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type selectProductSnapshot struct {
	Name     string
	Category string
	Price    float64
}

var selectProductMeta = drel.ModelMeta[selectProduct]{
	Table:    "products",
	Columns:  []string{"id", "name", "category", "price", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*selectProduct, error) {
		p := &selectProduct{}
		err := row.Scan(&p.ID, &p.Name, &p.Category, &p.Price, &p.CreatedAt, &p.UpdatedAt)
		return p, err
	},
	Snapshot: func(p *selectProduct) any {
		return selectProductSnapshot{Name: p.Name, Category: p.Category, Price: p.Price}
	},
	Diff: func(p *selectProduct, snap any) []drel.FieldChange {
		s := snap.(selectProductSnapshot)
		var changes []drel.FieldChange
		if p.Name != s.Name {
			changes = append(changes, drel.FieldChange{Column: "name", Value: p.Name})
		}
		if p.Category != s.Category {
			changes = append(changes, drel.FieldChange{Column: "category", Value: p.Category})
		}
		if p.Price != s.Price {
			changes = append(changes, drel.FieldChange{Column: "price", Value: p.Price})
		}
		return changes
	},
	PKValue: func(p *selectProduct) any { return p.ID },
	InsertColumns: func(p *selectProduct) ([]string, []any) {
		return []string{"name", "category", "price"},
			[]any{p.Name, p.Category, p.Price}
	},
	ScanReturning: func(p *selectProduct, row drel.Row) error {
		return row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	},
}

func setupSelectEngine(t *testing.T) (*drel.Engine, *drel.Repository[selectProduct]) {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `
		CREATE TABLE products (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			category   TEXT NOT NULL,
			price      REAL NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Seed data.
	_, err = engine.Exec(ctx, "INSERT INTO products (name, category, price) VALUES (?, ?, ?)", "Widget A", "electronics", 9.99)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO products (name, category, price) VALUES (?, ?, ?)", "Widget B", "electronics", 19.99)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO products (name, category, price) VALUES (?, ?, ?)", "Gadget C", "accessories", 5.99)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO products (name, category, price) VALUES (?, ?, ?)", "Gadget D", "accessories", 14.99)
	require.NoError(t, err)

	repo := drel.NewRepository(engine, selectProductMeta)
	return engine, repo
}

// ─── Select projection ─────────────────────────────────────────────────────

type nameOnlyDTO struct {
	Name string `db:"name"`
}

func TestSelect_Projection(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.OrderBy(drel.NewStringCol("name").Asc())
	results, err := drel.Select[nameOnlyDTO](ctx, qb, drel.ColRef("name"))
	require.NoError(t, err)
	require.Len(t, results, 4)

	assert.Equal(t, "Gadget C", results[0].Name)
	assert.Equal(t, "Gadget D", results[1].Name)
	assert.Equal(t, "Widget A", results[2].Name)
	assert.Equal(t, "Widget B", results[3].Name)
}

type namePriceDTO struct {
	Name  string  `db:"name"`
	Price float64 `db:"price"`
}

func TestSelect_ProjectionWithFilter(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	priceCol := drel.NewOrderedCol[float64]("price")
	qb := repo.Where(priceCol.GT(10.0)).OrderBy(priceCol.Asc())

	results, err := drel.Select[namePriceDTO](ctx, qb,
		drel.ColRef("name"), drel.ColRef("price"))
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "Gadget D", results[0].Name)
	assert.InDelta(t, 14.99, results[0].Price, 0.001)
	assert.Equal(t, "Widget B", results[1].Name)
	assert.InDelta(t, 19.99, results[1].Price, 0.001)
}

// ─── Aggregate ──────────────────────────────────────────────────────────────

func TestAggregate_Sum(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.Where(drel.NewStringCol("category").Eq("electronics"))
	total, err := drel.Aggregate[float64](ctx, qb, drel.Sum(drel.ColRef("price")))
	require.NoError(t, err)
	assert.InDelta(t, 29.98, total, 0.001)
}

func TestAggregate_Max(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.Where(drel.NewStringCol("category").Eq("accessories"))
	maxPrice, err := drel.Aggregate[float64](ctx, qb, drel.Max(drel.ColRef("price")))
	require.NoError(t, err)
	assert.InDelta(t, 14.99, maxPrice, 0.001)
}

func TestAggregate_Min(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.Where(drel.NewStringCol("category").Eq("accessories"))
	minPrice, err := drel.Aggregate[float64](ctx, qb, drel.Min(drel.ColRef("price")))
	require.NoError(t, err)
	assert.InDelta(t, 5.99, minPrice, 0.001)
}

func TestAggregate_Count(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.Where(drel.NewStringCol("category").Eq("electronics"))
	count, err := drel.Aggregate[int](ctx, qb, drel.CountCol(drel.ColRef("id")))
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// ─── GroupBy ────────────────────────────────────────────────────────────────

type categoryStatsDTO struct {
	Category   string  `db:"category"`
	TotalPrice float64 `db:"total_price"`
}

func TestGroupBy_Basic(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.OrderBy(drel.NewStringCol("category").Asc())
	results, err := drel.GroupBy[categoryStatsDTO](ctx, qb,
		[]drel.GroupSpec{drel.Group(drel.ColRef("category"))},
		[]drel.AliasedAgg{drel.As("total_price", drel.Sum(drel.ColRef("price")))},
	)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "accessories", results[0].Category)
	assert.InDelta(t, 20.98, results[0].TotalPrice, 0.001)
	assert.Equal(t, "electronics", results[1].Category)
	assert.InDelta(t, 29.98, results[1].TotalPrice, 0.001)
}

func TestGroupBy_WithHaving(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.OrderBy(drel.NewStringCol("category").Asc())
	results, err := drel.GroupBy[categoryStatsDTO](ctx, qb,
		[]drel.GroupSpec{drel.Group(drel.ColRef("category"))},
		[]drel.AliasedAgg{drel.As("total_price", drel.Sum(drel.ColRef("price")))},
		drel.Having(drel.Raw("SUM(price) > ?", 25.0)),
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "electronics", results[0].Category)
	assert.InDelta(t, 29.98, results[0].TotalPrice, 0.001)
}

// ─── ColRef on column types ─────────────────────────────────────────────────

func TestColumn_ColRef(t *testing.T) {
	col := drel.NewCol[string]("email")
	ref := col.ColRef()
	assert.Equal(t, "email", ref.Name())
}

func TestOrderedColumn_ColRef(t *testing.T) {
	col := drel.NewOrderedCol[int]("age")
	ref := col.ColRef()
	assert.Equal(t, "age", ref.Name())
}

func TestStringColumn_ColRef(t *testing.T) {
	col := drel.NewStringCol("name")
	ref := col.ColRef()
	assert.Equal(t, "name", ref.Name())
}

func TestBoolColumn_ColRef(t *testing.T) {
	col := drel.NewBoolCol("active")
	ref := col.ColRef()
	assert.Equal(t, "active", ref.Name())
}

// DTO declares fields in the REVERSE of the projection order. With positional
// binding the price string lands in Name and vice-versa; with name binding each
// value lands in its tagged field.
type priceNameDTO struct {
	Price float64 `db:"price"`
	Name  string  `db:"name"`
}

func TestSelect_BindsByNameNotStructOrder(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	priceCol := drel.NewOrderedCol[float64]("price")
	qb := repo.Where(priceCol.GT(10.0)).OrderBy(priceCol.Asc())

	// Projection order (name, price) differs from struct order (Price, Name).
	results, err := drel.Select[priceNameDTO](ctx, qb,
		drel.ColRef("name"), drel.ColRef("price"))
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "Gadget D", results[0].Name)
	assert.InDelta(t, 14.99, results[0].Price, 0.001)
	assert.Equal(t, "Widget B", results[1].Name)
	assert.InDelta(t, 19.99, results[1].Price, 0.001)
}

func TestSelect_UnknownColumnFailsLoudly(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.OrderBy(drel.NewStringCol("name").Asc())
	// "price" has no field in nameOnlyDTO.
	_, err := drel.Select[nameOnlyDTO](ctx, qb,
		drel.ColRef("name"), drel.ColRef("price"))
	require.Error(t, err)
	assert.ErrorIs(t, err, drel.ErrUnknownProjectionColumn)
}

// DTO declares the aggregate alias BEFORE the group column — the reverse of the
// emit order (group col, then alias). Positional binding swaps them; name
// binding does not.
type statsCategoryDTO struct {
	TotalPrice float64 `db:"total_price"`
	Category   string  `db:"category"`
}

func TestGroupBy_BindsByNameNotStructOrder(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.OrderBy(drel.NewStringCol("category").Asc())
	results, err := drel.GroupBy[statsCategoryDTO](ctx, qb,
		[]drel.GroupSpec{drel.Group(drel.ColRef("category"))},
		[]drel.AliasedAgg{drel.As("total_price", drel.Sum(drel.ColRef("price")))},
	)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "accessories", results[0].Category)
	assert.InDelta(t, 20.98, results[0].TotalPrice, 0.001)
	assert.Equal(t, "electronics", results[1].Category)
	assert.InDelta(t, 29.98, results[1].TotalPrice, 0.001)
}

func TestGroupBy_UnknownColumnFailsLoudly(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.OrderBy(drel.NewStringCol("category").Asc())
	// Alias "wrong_alias" has no matching field in categoryStatsDTO
	// (which tags total_price), so the projected alias column is unknown.
	_, err := drel.GroupBy[categoryStatsDTO](ctx, qb,
		[]drel.GroupSpec{drel.Group(drel.ColRef("category"))},
		[]drel.AliasedAgg{drel.As("wrong_alias", drel.Sum(drel.ColRef("price")))},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, drel.ErrUnknownProjectionColumn)
}

func TestSelect_UnknownColumn_EmptyResultStillFailsLoudly(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	// Filter matches no rows (max seeded price is 19.99).
	priceCol := drel.NewOrderedCol[float64]("price")
	qb := repo.Where(priceCol.GT(1000.0))

	// "price" has no field in nameOnlyDTO — must fail even with zero rows.
	_, err := drel.Select[nameOnlyDTO](ctx, qb,
		drel.ColRef("name"), drel.ColRef("price"))
	require.Error(t, err)
	assert.ErrorIs(t, err, drel.ErrUnknownProjectionColumn)
}

func TestGroupBy_UnknownColumn_EmptyResultStillFailsLoudly(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	// Filter matches no rows, so the GROUP BY produces zero groups.
	priceCol := drel.NewOrderedCol[float64]("price")
	qb := repo.Where(priceCol.GT(1000.0))

	// Alias "wrong_alias" has no matching field in categoryStatsDTO.
	_, err := drel.GroupBy[categoryStatsDTO](ctx, qb,
		[]drel.GroupSpec{drel.Group(drel.ColRef("category"))},
		[]drel.AliasedAgg{drel.As("wrong_alias", drel.Sum(drel.ColRef("price")))},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, drel.ErrUnknownProjectionColumn)
}

type categoryOnlyDTO struct {
	Category string `db:"category"`
}

func TestSelect_Distinct(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()

	qb := repo.Distinct().OrderBy(drel.NewStringCol("category").Asc())
	results, err := drel.Select[categoryOnlyDTO](ctx, qb, drel.ColRef("category"))
	require.NoError(t, err)
	require.Len(t, results, 2) // 4 rows, 2 distinct categories
	assert.Equal(t, "accessories", results[0].Category)
	assert.Equal(t, "electronics", results[1].Category)
}
