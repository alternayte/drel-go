package testmodels

import (
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

type productSnapshot struct {
	Name    string
	Price   int
	InStock bool
}

var ProductMeta = drel.ModelMeta[Product]{
	Table:    "products",
	Columns:  []string{"id", "name", "price", "in_stock", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*Product, error) {
		p := &Product{}
		err := row.Scan(&p.ID, &p.Name, &p.Price, &p.InStock, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return p, nil
	},
	Snapshot: func(p *Product) any {
		return productSnapshot{Name: p.Name, Price: p.Price, InStock: p.InStock}
	},
	Diff: func(p *Product, snap any) []drel.FieldChange {
		s := snap.(productSnapshot)
		var changes []drel.FieldChange
		if p.Name != s.Name {
			changes = append(changes, drel.FieldChange{Column: "name", Value: p.Name})
		}
		if p.Price != s.Price {
			changes = append(changes, drel.FieldChange{Column: "price", Value: p.Price})
		}
		if p.InStock != s.InStock {
			changes = append(changes, drel.FieldChange{Column: "in_stock", Value: p.InStock})
		}
		return changes
	},
	PKValue: func(p *Product) any {
		return p.ID
	},
	InsertColumns: func(p *Product) ([]string, []any) {
		return []string{"name", "price", "in_stock"}, []any{p.Name, p.Price, p.InStock}
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
