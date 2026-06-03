package catalog

import "github.com/alternayte/drel"

// Product is a minimal model used to demonstrate drel's observability hooks:
// structured query logging, tracing spans, and dev-mode diagnostics.
type Product struct {
	drel.Model[int]
	Name  string `db:"name"`
	Price int    `db:"price"`
}

func NewProduct(name string, price int) *Product {
	return &Product{Name: name, Price: price}
}
