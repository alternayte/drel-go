package drel

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type scanField struct {
	column string
	index  int
}

type scanPlan struct {
	fields []scanField
	// byColumn maps a db-tag column name to the destination's index within
	// fields (the same slice scanDest iterates). It lets scanDestFor bind
	// projected SQL columns to DTO fields by name instead of by struct order.
	byColumn map[string]int
	isScalar bool  // true when T is a non-struct scalar scanned into one dest
	err      error // non-nil when T is unsupported (e.g. a map/slice/chan)
}

var (
	scanPlanMu    sync.RWMutex
	scanPlanCache = make(map[reflect.Type]*scanPlan)
)

func getScanPlan(t reflect.Type) *scanPlan {
	scanPlanMu.RLock()
	plan, ok := scanPlanCache[t]
	scanPlanMu.RUnlock()
	if ok {
		return plan
	}
	plan = buildScanPlan(t)
	scanPlanMu.Lock()
	scanPlanCache[t] = plan
	scanPlanMu.Unlock()
	return plan
}

func buildScanPlan(t reflect.Type) *scanPlan {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		// []byte is a scannable scalar (e.g. for BLOB columns).
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &scanPlan{isScalar: true}
		}
		// A scalar (or other single-value) destination is supported only for the
		// kinds database/sql / pgx can scan into directly. Structs are handled by
		// db-tagged fields above; maps/slices/funcs/chans are not scannable.
		switch t.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.Func, reflect.Chan, reflect.Invalid:
			return &scanPlan{err: fmt.Errorf("requires a struct with db tags or a scalar column type; got %s", t.Kind())}
		default:
			return &scanPlan{isScalar: true}
		}
	}
	var fields []scanField
	byColumn := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		byColumn[tag] = len(fields)
		fields = append(fields, scanField{column: tag, index: i})
	}
	return &scanPlan{fields: fields, byColumn: byColumn}
}

func (p *scanPlan) columns() []string {
	cols := make([]string, len(p.fields))
	for i, f := range p.fields {
		cols[i] = f.column
	}
	return cols
}

func (p *scanPlan) scanDest(v reflect.Value) []any {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if p.isScalar {
		return []any{v.Addr().Interface()}
	}
	dests := make([]any, len(p.fields))
	for i, f := range p.fields {
		dests[i] = v.Field(f.index).Addr().Interface()
	}
	return dests
}

// colKey returns the lookup key for a (possibly table-qualified) column name.
// A bare column "name" maps to itself; a qualified "products.name" maps to "name"
// (the unqualified part) when the qualified form has no direct match. This lets
// QualifiedColRef("products","name").Ref() bind to a DTO field tagged `db:"name"`.
func (p *scanPlan) colKey(col string) string {
	if _, ok := p.byColumn[col]; ok {
		return col
	}
	if i := strings.LastIndex(col, "."); i >= 0 {
		unqualified := col[i+1:]
		if _, ok := p.byColumn[unqualified]; ok {
			return unqualified
		}
	}
	return col // caller will detect the missing key and return ErrUnknownProjectionColumn
}

// validateColumns returns ErrUnknownProjectionColumn for the first requested
// column that has no matching db-tagged DTO field. Select/GroupBy call this
// before executing the query so an unknown projected column fails loudly even
// when the result set is empty (scanDestFor only runs per row).
func (p *scanPlan) validateColumns(columns []string) error {
	for _, col := range columns {
		if _, ok := p.byColumn[p.colKey(col)]; !ok {
			return fmt.Errorf("%w: %q", ErrUnknownProjectionColumn, col)
		}
	}
	return nil
}

// scanDestFor returns scan destinations ordered to match the given SQL column
// names (the emit order of the SELECT). Each column is bound by name to the DTO
// field whose `db` tag equals it, so the SELECT-emit order and the scan order
// are derived from the same column list and can never diverge.
//
// A requested column with no matching field is a loud ErrUnknownProjectionColumn
// rather than a silent misbind — silently dropping a column is how the original
// positional-binding corruption hid.
func (p *scanPlan) scanDestFor(v reflect.Value, columns []string) ([]any, error) {
	if err := p.validateColumns(columns); err != nil {
		return nil, err
	}
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	dests := make([]any, len(columns))
	for i, col := range columns {
		dests[i] = v.Field(p.fields[p.byColumn[p.colKey(col)]].index).Addr().Interface()
	}
	return dests, nil
}
