package drel

import (
	"context"
	"fmt"
	"reflect"
)

// RawQuery executes arbitrary SQL and scans results into []*T.
// Uses $1, $2 placeholder format — rewritten to ? for SQLite automatically.
func RawQuery[T any](ctx context.Context, e *Engine, sql string, args ...any) ([]*T, error) {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}

	plan := getScanPlan(reflect.TypeOf((*T)(nil)).Elem())

	rows, err := e.queryInternal(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rt := reflect.TypeOf((*T)(nil)).Elem()
	var items []*T
	for rows.Next() {
		v := reflect.New(rt)
		dests := plan.scanDest(v)
		if err := rows.Scan(dests...); err != nil {
			return nil, fmt.Errorf("drel: raw query scan: %w", err)
		}
		items = append(items, v.Interface().(*T))
	}
	return items, rows.Err()
}

// RawQueryRow executes arbitrary SQL expecting exactly one row, scanned into *T.
// Uses $1, $2 placeholder format — rewritten to ? for SQLite automatically.
func RawQueryRow[T any](ctx context.Context, e *Engine, sql string, args ...any) (*T, error) {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}

	plan := getScanPlan(reflect.TypeOf((*T)(nil)).Elem())
	row := e.queryRowInternal(ctx, sql, args...)

	rt := reflect.TypeOf((*T)(nil)).Elem()
	v := reflect.New(rt)
	dests := plan.scanDest(v)
	if err := row.Scan(dests...); err != nil {
		return nil, fmt.Errorf("drel: raw query row scan: %w", err)
	}
	return v.Interface().(*T), nil
}
