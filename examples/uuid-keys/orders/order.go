package orders

import (
	"github.com/alternayte/drel"
	"github.com/google/uuid"
)

// Order uses an application-assigned UUIDv7 primary key. drel stamps a fresh
// v7 UUID at Add() time — the id is available before any flush.
type Order struct {
	drel.Model[uuid.UUID]
	Customer string `db:"customer"`
	Total    int    `db:"total"`
}

func NewOrder(customer string, total int) *Order {
	return &Order{Customer: customer, Total: total}
}
