package drel

import (
	"fmt"
	"reflect"
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
	dests := make([]any, len(p.fields))
	for i, f := range p.fields {
		dests[i] = v.Field(f.index).Addr().Interface()
	}
	return dests
}

// validateColumns returns ErrUnknownProjectionColumn for the first requested
// column that has no matching db-tagged DTO field. Select/GroupBy call this
// before executing the query so an unknown projected column fails loudly even
// when the result set is empty (scanDestFor only runs per row).
func (p *scanPlan) validateColumns(columns []string) error {
	for _, col := range columns {
		if _, ok := p.byColumn[col]; !ok {
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
		dests[i] = v.Field(p.fields[p.byColumn[col]].index).Addr().Interface()
	}
	return dests, nil
}
