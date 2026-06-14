package drel

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"time"

	"github.com/alternayte/drel/internal/ast"
)

// ErrCursorPaginationNeedsOrderBy is returned when Page is called without an OrderBy.
// Keyset pagination requires a deterministic ordering.
var ErrCursorPaginationNeedsOrderBy = errors.New("drel: cursor pagination requires OrderBy")

// ErrPaginationNeedsLimit is returned when a pagination method is called without Take/Limit.
var ErrPaginationNeedsLimit = errors.New("drel: pagination requires Take/Limit to set the page size")

// ErrInvalidCursor is returned when a cursor string cannot be decoded.
var ErrInvalidCursor = errors.New("drel: invalid cursor")

// ErrInvalidPageSize is returned when a pagination page size (Take/Limit) is <= 0.
var ErrInvalidPageSize = errors.New("drel: page size (Take/Limit) must be > 0")

// OffsetPage holds a page of results from offset-based pagination.
type OffsetPage[T any] struct {
	Items      []*T
	Total      int
	Page       int // 1-based page number
	PageSize   int
	TotalPages int
	HasMore    bool
}

// CursorPage holds a page of results from keyset (cursor) pagination.
type CursorPage[T any] struct {
	Items      []*T
	NextCursor string // empty when there are no more results
	HasMore    bool
}

func init() {
	// Register the concrete types that can appear as ordering-key values so they
	// survive gob round-tripping inside an opaque cursor.
	gob.Register(int(0))
	gob.Register(int8(0))
	gob.Register(int16(0))
	gob.Register(int32(0))
	gob.Register(int64(0))
	gob.Register(uint(0))
	gob.Register(uint8(0))
	gob.Register(uint16(0))
	gob.Register(uint32(0))
	gob.Register(uint64(0))
	gob.Register(float32(0))
	gob.Register(float64(0))
	gob.Register("")
	gob.Register(false)
	gob.Register([]byte(nil))
	gob.Register(time.Time{})
}

// cursorPayload is the gob-encoded contents of a cursor string. Columns is stored
// alongside Vals so a cursor cannot be silently applied to a mismatched ordering.
type cursorPayload struct {
	Columns []string
	Vals    []any
}

func encodeCursor(columns []string, vals []any) (string, error) {
	// Register each value's concrete type so gob can encode named types
	// (enums, uuid.UUID, value objects) behind the []any. Registration is
	// process-global and idempotent for a given type; subsequent decodes in the
	// same process therefore succeed. The static init() registrations cover the
	// built-in kinds for decode-before-encode robustness across restarts.
	for _, v := range vals {
		registerCursorType(v)
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(cursorPayload{Columns: columns, Vals: vals}); err != nil {
		return "", fmt.Errorf("drel: encode cursor (order-key type may be unsupported): %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// registerCursorType registers v's concrete type with gob, swallowing the
// benign panic gob raises if a different type was already registered under the
// same name (extremely unlikely for ordering-key types).
func registerCursorType(v any) {
	if v == nil {
		return
	}
	defer func() { _ = recover() }()
	gob.Register(v)
}

func decodeCursor(s string) (cursorPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursorPayload{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	var p cursorPayload
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&p); err != nil {
		return cursorPayload{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	return p, nil
}

// keysetClause builds the WHERE predicate for keyset pagination given an ordering
// and the cursor values of the last row of the previous page. For ORDER BY
// (c1 d1, c2 d2, ...) the predicate is the lexicographic row comparison:
//
//	(c1 OP1 v1)
//	OR (c1 = v1 AND c2 OP2 v2)
//	OR (c1 = v1 AND c2 = v2 AND c3 OP3 v3) ...
//
// where OPk is ">" for ascending and "<" for descending columns.
func keysetClause(order []ast.OrderByExpr, vals []any) ast.WhereClause {
	var orTerms []ast.WhereClause
	for k := range order {
		var andTerms []ast.WhereClause
		for j := 0; j < k; j++ {
			andTerms = append(andTerms, ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: order[j].Column, Op: ast.OpEq, Value: vals[j]},
			})
		}
		op := ast.OpGT
		if order[k].Direction == ast.Desc {
			op = ast.OpLT
		}
		andTerms = append(andTerms, ast.WhereClause{
			Comparison: &ast.ComparisonNode{Column: order[k].Column, Op: op, Value: vals[k]},
		})
		if len(andTerms) == 1 {
			orTerms = append(orTerms, andTerms[0])
		} else {
			orTerms = append(orTerms, ast.WhereClause{LogicalOp: ast.LogicalAnd, Children: andTerms})
		}
	}
	if len(orTerms) == 1 {
		return orTerms[0]
	}
	return ast.WhereClause{LogicalOp: ast.LogicalOr, Children: orTerms}
}

// cursorOrder returns the effective ordering for cursor pagination: the builder's
// OrderBy with the primary key appended as a final tiebreaker if not already
// present, guaranteeing a total order so the keyset never skips or repeats rows.
func cursorOrder(orderBy []ast.OrderByExpr, pkColumn string) []ast.OrderByExpr {
	for _, o := range orderBy {
		if o.Column == pkColumn {
			return orderBy
		}
	}
	out := make([]ast.OrderByExpr, len(orderBy), len(orderBy)+1)
	copy(out, orderBy)
	return append(out, ast.OrderByExpr{Column: pkColumn, Direction: ast.Asc})
}

// buildOffsetPage assembles an OffsetPage from a fetched slice and the total count.
func buildOffsetPage[T any](items []*T, total, pageSize, offset int) *OffsetPage[T] {
	page := 1
	totalPages := 0
	if pageSize > 0 {
		page = offset/pageSize + 1
		totalPages = (total + pageSize - 1) / pageSize
	}
	return &OffsetPage[T]{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		HasMore:    offset+len(items) < total,
	}
}

// validateCursorColumns ensures a decoded cursor matches the current ordering,
// preventing a cursor minted under one OrderBy from being misapplied to another.
func validateCursorColumns(p cursorPayload, order []ast.OrderByExpr) error {
	if len(p.Columns) != len(order) || len(p.Vals) != len(order) {
		return fmt.Errorf("%w: cursor does not match the current ordering", ErrInvalidCursor)
	}
	for i := range order {
		if p.Columns[i] != order[i].Column {
			return fmt.Errorf("%w: cursor was created for a different ordering", ErrInvalidCursor)
		}
	}
	return nil
}

// finishCursorPage trims the over-fetched row, determines HasMore, and encodes
// the NextCursor from the last returned item's ordering-key values.
func finishCursorPage[T any](meta *ModelMeta[T], order []ast.OrderByExpr, items []*T, pageSize int) (*CursorPage[T], error) {
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	page := &CursorPage[T]{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		vals, err := extractCursorVals(meta, order, last)
		if err != nil {
			return nil, err
		}
		cols := make([]string, len(order))
		for i, o := range order {
			cols[i] = o.Column
		}
		cursor, err := encodeCursor(cols, vals)
		if err != nil {
			return nil, err
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// extractCursorVals reads the ordering-key column values from an entity, in the
// order columns appear in `order`.
func extractCursorVals[T any](meta *ModelMeta[T], order []ast.OrderByExpr, entity *T) ([]any, error) {
	if meta.ColumnValue == nil {
		return nil, fmt.Errorf("drel: cursor pagination on %s requires generated ColumnValue accessor", meta.Table)
	}
	vals := make([]any, len(order))
	for i, o := range order {
		idx := findColumnIndex(meta.Columns, o.Column)
		if idx < 0 {
			return nil, fmt.Errorf("drel: cursor pagination: order column %q not found on %s", o.Column, meta.Table)
		}
		vals[i] = meta.ColumnValue(entity, idx)
	}
	return vals, nil
}
