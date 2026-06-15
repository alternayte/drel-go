package drel

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// OutboxMessage is a row written to the transactional outbox. Type identifies
// the event kind; Payload is JSON-serialized into the outbox table's payload
// column. Messages are written within the same transaction as the changes that
// produced the events, giving exactly-once hand-off to an external relay
// (Debezium CDC, a polling worker, etc.).
type OutboxMessage struct {
	Type    string
	Payload any
}

type outboxConfig struct {
	table  string
	mapper func(event any) (OutboxMessage, bool)
}

// OutboxOption configures UseOutbox.
type OutboxOption func(*outboxConfig)

// WithOutboxMapper customizes how domain events are mapped to outbox messages.
// Return ok=false to skip an event. The default maps every event to a message
// whose Type is the event's Go type name and whose Payload is the event itself.
func WithOutboxMapper(fn func(event any) (OutboxMessage, bool)) OutboxOption {
	return func(c *outboxConfig) { c.mapper = fn }
}

// UseOutbox writes domain events to a transactional outbox table on every
// SaveChanges, within the same transaction as the changes. The table must exist
// (see OutboxSchema) with at least (type, payload) columns; payload is stored as
// JSON text. Events are dispatched to after-commit handlers as usual in addition
// to being persisted.
func (e *Engine) UseOutbox(table string, opts ...OutboxOption) {
	cfg := &outboxConfig{table: table, mapper: defaultOutboxMapper}
	for _, o := range opts {
		o(cfg)
	}
	e.addEventSink(func(ctx context.Context, tx *Tx, events []any) error {
		for _, ev := range events {
			msg, ok := cfg.mapper(ev)
			if !ok {
				continue
			}
			payload, err := json.Marshal(msg.Payload)
			if err != nil {
				return fmt.Errorf("drel: outbox marshal %s: %w", msg.Type, err)
			}
			res := e.dia.BuildInsert(cfg.table, []string{"type", "payload"},
				[]any{msg.Type, string(payload)}, nil)
			if _, err := tx.Exec(ctx, res.SQL, res.Args...); err != nil {
				return fmt.Errorf("drel: outbox insert into %s: %w", cfg.table, err)
			}
		}
		return nil
	})
}

func defaultOutboxMapper(ev any) (OutboxMessage, bool) {
	return OutboxMessage{Type: eventTypeName(ev), Payload: ev}, true
}

// eventTypeName returns the unqualified Go type name of an event value.
func eventTypeName(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// OutboxSchema returns CREATE TABLE (and a partial index) DDL for an outbox
// table for the given dialect ("postgres" or "sqlite"). The payload column is
// TEXT for portability (JSON is stored as a string); processed_at is left for a
// relay to stamp.
//
// A partial index on unprocessed rows is emitted so the canonical relay poll
//
//	SELECT id, type, payload FROM <table> WHERE processed_at IS NULL ORDER BY id;
//
// uses an index seek rather than a sequential scan that grows with total outbox
// size (processed rows are typically retained for audit).
func OutboxSchema(table, dialect string) string {
	q := `"` + table + `"`
	idx := `"idx_` + table + `_unprocessed"`
	index := fmt.Sprintf(
		`CREATE INDEX %s ON %s ("id") WHERE "processed_at" IS NULL;`+"\n", idx, q)
	if dialect == "sqlite" {
		return fmt.Sprintf(`CREATE TABLE %s (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "type" TEXT NOT NULL,
    "payload" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "processed_at" DATETIME
);
%s`, q, index)
	}
	return fmt.Sprintf(`CREATE TABLE %s (
    "id" BIGSERIAL PRIMARY KEY,
    "type" TEXT NOT NULL,
    "payload" TEXT NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "processed_at" TIMESTAMPTZ
);
%s`, q, index)
}
