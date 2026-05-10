// Example: transactions
//
// Demonstrates drel's transaction-scoped change tracking:
// insert, update (only changed fields), delete, rollback on error.
//
// Usage:
//
//	export DATABASE_URL="postgres://localhost:5432/drelexample?sslmode=disable"
//	go run ./examples/transactions/
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
	Snapshot: func(p *Product) any {
		return [3]any{p.Name, p.Price, p.InStock}
	},
	Diff: func(p *Product, snap any) []drel.FieldChange {
		s := snap.([3]any)
		var changes []drel.FieldChange
		if p.Name != s[0].(string) {
			changes = append(changes, drel.FieldChange{Column: "name", Value: p.Name})
		}
		if p.Price != s[1].(int) {
			changes = append(changes, drel.FieldChange{Column: "price", Value: p.Price})
		}
		if p.InStock != s[2].(bool) {
			changes = append(changes, drel.FieldChange{Column: "in_stock", Value: p.InStock})
		}
		return changes
	},
	PKValue:       func(p *Product) any { return p.ID },
	InsertColumns: func(p *Product) ([]string, []any) { return []string{"name", "price", "in_stock"}, []any{p.Name, p.Price, p.InStock} },
	ScanReturning: func(p *Product, row drel.Row) error { return row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt) },
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

	// === INSERT ===
	fmt.Println("=== Insert via transaction ===")
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		txRepo := drel.NewTxRepository(tx, ProductMeta)
		laptop := &Product{Name: "Laptop", Price: 99900, InStock: true}
		txRepo.Add(laptop)
		return nil // auto-flush + commit
	})
	if err != nil {
		log.Fatal(err)
	}

	all, _ := repo.All(ctx)
	fmt.Printf("  Products after insert: %d\n", len(all))
	for _, p := range all {
		fmt.Printf("    [%d] %s — $%d\n", p.ID, p.Name, p.Price)
	}

	// === UPDATE (only changed fields) ===
	fmt.Println("\n=== Update price only (change tracking) ===")
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		txRepo := drel.NewTxRepository(tx, ProductMeta)
		laptop, err := txRepo.Find(ctx, 1)
		if err != nil {
			return err
		}
		fmt.Printf("  Before: %s — $%d\n", laptop.Name, laptop.Price)
		laptop.Price = 89900 // only this field changes
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	updated, _ := repo.Find(ctx, 1)
	fmt.Printf("  After:  %s — $%d\n", updated.Name, updated.Price)

	// === DELETE ===
	fmt.Println("\n=== Delete via transaction ===")
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		txRepo := drel.NewTxRepository(tx, ProductMeta)
		laptop, err := txRepo.Find(ctx, 1)
		if err != nil {
			return err
		}
		return txRepo.Remove(laptop)
	})
	if err != nil {
		log.Fatal(err)
	}

	count, _ := repo.Count(ctx)
	fmt.Printf("  Products after delete: %d\n", count)

	// === ROLLBACK ===
	fmt.Println("\n=== Rollback on error ===")

	// Insert a product first
	engine.Transaction(ctx, func(tx *drel.Tx) error {
		txRepo := drel.NewTxRepository(tx, ProductMeta)
		txRepo.Add(&Product{Name: "Tablet", Price: 49900, InStock: true})
		return nil
	})

	countBefore, _ := repo.Count(ctx)
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		txRepo := drel.NewTxRepository(tx, ProductMeta)
		txRepo.Add(&Product{Name: "Ghost Product", Price: 1, InStock: false})
		return fmt.Errorf("something went wrong") // triggers rollback
	})
	fmt.Printf("  Error: %v\n", err)

	countAfter, _ := repo.Count(ctx)
	fmt.Printf("  Products before: %d, after rollback: %d (unchanged)\n", countBefore, countAfter)

	// === MID-TX SAVECHANGES ===
	fmt.Println("\n=== Mid-transaction SaveChanges (get generated ID) ===")
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		txRepo := drel.NewTxRepository(tx, ProductMeta)

		phone := &Product{Name: "Phone", Price: 79900, InStock: true}
		txRepo.Add(phone)
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		fmt.Printf("  Phone inserted with ID: %d\n", phone.ID)

		case_ := &Product{Name: "Phone Case", Price: 2900, InStock: true}
		txRepo.Add(case_)
		return nil // second flush + commit
	})
	if err != nil {
		log.Fatal(err)
	}

	final, _ := repo.All(ctx)
	fmt.Printf("  Final product count: %d\n", len(final))
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
}

func teardown(ctx context.Context, engine *drel.Engine) {
	engine.Driver().Exec(ctx, `DROP TABLE IF EXISTS products`)
}
