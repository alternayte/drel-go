package drel

import (
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
