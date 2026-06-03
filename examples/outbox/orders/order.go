package orders

import "github.com/alternayte/drel"

// Domain events. Each is recorded by an Order method and, on commit, written to
// the transactional outbox table by drel.Engine.UseOutbox. The default outbox
// mapper turns each event into a row whose `type` is the Go type name
// ("OrderPlaced", "OrderShipped") and whose `payload` is the event marshalled
// to JSON.

// OrderPlaced is recorded when a new order is accepted.
type OrderPlaced struct {
	OrderID  int    `json:"order_id"`
	Customer string `json:"customer"`
	Total    int    `json:"total_cents"`
}

// OrderShipped is recorded when an order is dispatched to a carrier.
type OrderShipped struct {
	OrderID int    `json:"order_id"`
	Carrier string `json:"carrier"`
}

// Order is a SQLite-backed model that raises domain events from its behaviour.
// Embedding drel.Model[int] provides ID(), RecordEvent and the change-tracking
// hooks the outbox relies on.
type Order struct {
	drel.Model[int]
	Customer string `db:"customer"`
	Total    int    `db:"total"`
	Status   string `db:"status,index"`
}

// NewOrder builds a placed-but-unsaved order. Total is in cents.
func NewOrder(customer string, total int) *Order {
	return &Order{Customer: customer, Total: total, Status: "placed"}
}

// Place records an OrderPlaced event. Call it after the order has been flushed
// (so ID() is populated) — see main.go for the ordering.
func (o *Order) Place() {
	o.RecordEvent(OrderPlaced{OrderID: o.ID(), Customer: o.Customer, Total: o.Total})
}

// Ship marks the order shipped and records an OrderShipped event.
func (o *Order) Ship(carrier string) {
	o.Status = "shipped"
	o.RecordEvent(OrderShipped{OrderID: o.ID(), Carrier: carrier})
}
