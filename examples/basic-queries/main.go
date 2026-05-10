// Example: basic-queries
//
// Demonstrates connecting to Postgres and running type-safe queries
// using drel's Repository and QueryBuilder.
//
// Usage:
//
//	export DATABASE_URL="postgres://localhost:5432/drelexample?sslmode=disable"
//	go run ./examples/basic-queries/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alternayte/drel"
)

type Product struct {
	ID        int
	Name      string
	Price     int
	InStock   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

var ProductMeta = drel.ModelMeta[Product]{
	Table:    "products",
	Columns:  []string{"id", "name", "price", "in_stock", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*Product, error) {
		p := &Product{}
		return p, row.Scan(&p.ID, &p.Name, &p.Price, &p.InStock, &p.CreatedAt, &p.UpdatedAt)
	},
}

var Products = struct {
	ID      drel.OrderedColumn[int]
	Name    drel.StringColumn
	Price   drel.OrderedColumn[int]
	InStock drel.BoolColumn
}{
	ID:      drel.NewOrderedCol[int]("id"),
	Name:    drel.NewStringCol("name"),
	Price:   drel.NewOrderedCol[int]("price"),
	InStock: drel.NewBoolCol("in_stock"),
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/drelexample?sslmode=disable"
	}

	ctx := context.Background()
	engine, err := drel.NewEngine(dsn, drel.WithContext(ctx))
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer engine.Close()

	setup(ctx, engine)
	defer teardown(ctx, engine)

	repo := drel.NewRepository(engine, ProductMeta)

	// Find by ID
	fmt.Println("=== Find by ID ===")
	p, err := repo.Find(ctx, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  %s — $%d\n", p.Name, p.Price)

	// Where + OrderBy + Limit
	fmt.Println("\n=== In-stock products, cheapest first (limit 3) ===")
	products, err := repo.
		Where(Products.InStock.IsTrue()).
		OrderBy(Products.Price.Asc()).
		Limit(3).
		All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range products {
		fmt.Printf("  %s — $%d\n", p.Name, p.Price)
	}

	// OR condition
	fmt.Println("\n=== Price < $600 OR price > $2900 ===")
	products, err = repo.
		Where(drel.Or(
			Products.Price.LT(600),
			Products.Price.GT(2900),
		)).
		All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range products {
		fmt.Printf("  %s — $%d (in stock: %v)\n", p.Name, p.Price, p.InStock)
	}

	// Count + Exists
	fmt.Println("\n=== Aggregates ===")
	count, _ := repo.Count(ctx)
	fmt.Printf("  Total products: %d\n", count)

	inStockCount, _ := repo.Where(Products.InStock.IsTrue()).Count(ctx)
	fmt.Printf("  In stock: %d\n", inStockCount)

	hasExpensive, _ := repo.Where(Products.Price.GT(5000)).Exists(ctx)
	fmt.Printf("  Has product > $5000: %v\n", hasExpensive)

	// String contains
	fmt.Println("\n=== Name contains 'get' ===")
	products, err = repo.Where(Products.Name.Contains("get")).All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range products {
		fmt.Printf("  %s\n", p.Name)
	}
}

func setup(ctx context.Context, engine *drel.Engine) {
	drv := engine.Driver()
	drv.Exec(ctx, `DROP TABLE IF EXISTS products`)
	drv.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price INTEGER NOT NULL,
			in_stock BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	for _, p := range []struct {
		name    string
		price   int
		inStock bool
	}{
		{"Widget", 1000, true},
		{"Gadget", 2500, true},
		{"Doohickey", 500, false},
		{"Thingamajig", 1500, true},
		{"Whatchamacallit", 3000, false},
	} {
		drv.Exec(ctx, `INSERT INTO products (name, price, in_stock) VALUES ($1, $2, $3)`, p.name, p.price, p.inStock)
	}
}

func teardown(ctx context.Context, engine *drel.Engine) {
	engine.Driver().Exec(ctx, `DROP TABLE IF EXISTS products`)
}
