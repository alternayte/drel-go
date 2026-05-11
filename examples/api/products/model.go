package products

import "github.com/alternayte/drel"

type Product struct {
	drel.Model[int]
	Name     string `db:"name"`
	Price    int    `db:"price"`
	Category string `db:"category"`
	InStock  bool   `db:"in_stock"`
}
