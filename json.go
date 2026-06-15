package drel

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a generic adapter that maps a Go value of type T to a JSON-encoded
// database column (Postgres jsonb / SQLite TEXT). It implements driver.Valuer
// (marshals *V on write) and sql.Scanner (unmarshals into *V on read), so it
// round-trips on both pgx and database/sql without any reflection in the ORM.
//
// Generated code wraps slice/map/struct columns as JSON{V: &p.Field}. V is a
// pointer so the value-receiver Scan can populate the underlying field.
type JSON[T any] struct {
	V *T
}

// Value marshals *V to JSON bytes. A nil pointer encodes as SQL NULL.
func (j JSON[T]) Value() (driver.Value, error) {
	if j.V == nil {
		return nil, nil
	}
	b, err := json.Marshal(*j.V)
	if err != nil {
		return nil, fmt.Errorf("drel: marshal JSON column: %w", err)
	}
	return b, nil
}

// Scan unmarshals a []byte or string source into *V. A nil source resets *V to
// its zero value.
func (j JSON[T]) Scan(src any) error {
	if j.V == nil {
		return fmt.Errorf("drel: JSON.Scan into nil pointer")
	}
	switch s := src.(type) {
	case nil:
		var zero T
		*j.V = zero
		return nil
	case []byte:
		return j.unmarshal(s)
	case string:
		return j.unmarshal([]byte(s))
	default:
		return fmt.Errorf("drel: cannot scan %T into JSON column", src)
	}
}

func (j JSON[T]) unmarshal(b []byte) error {
	if len(b) == 0 {
		var zero T
		*j.V = zero
		return nil
	}
	if err := json.Unmarshal(b, j.V); err != nil {
		return fmt.Errorf("drel: unmarshal JSON column: %w", err)
	}
	return nil
}

// JSONEqual reports whether a and b marshal to identical JSON. It is used by
// generated diff code for JSON struct/non-comparable columns that have no cheap
// slices.Equal/maps.Equal form. On a marshal error it conservatively reports
// false so the change is recorded (a write is safer than silently skipping it).
func JSONEqual[T any](a, b T) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}
