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
