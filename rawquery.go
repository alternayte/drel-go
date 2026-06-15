package drel

import (
	"context"
	"fmt"
	"reflect"
)

// RawQuery executes arbitrary SQL and scans results into []*T.
// Uses $1, $2 placeholder format — rewritten to ? for SQLite automatically.
//
// IMPORTANT: results are bound to T's fields in struct-declaration order (the
// `db`-tagged fields, top to bottom), NOT by matching result-set column names.
// You MUST write the SELECT column list in the same order as T declares its
// tagged fields, or values land in the wrong fields. Unlike Select/GroupBy,
// which bind by name, RawQuery cannot inspect the result-set column names today.
func RawQuery[T any](ctx context.Context, e *Engine, sql string, args ...any) ([]*T, error) {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}

	plan := getScanPlan(reflect.TypeOf((*T)(nil)).Elem())
	if plan.err != nil {
		var zero T
		return nil, fmt.Errorf("drel: RawQuery[%T] %w", zero, plan.err)
	}

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
//
// IMPORTANT: like RawQuery, the single row is bound to T's fields in
// struct-declaration order, NOT by result-set column name. Order the SELECT
// columns to match T's tagged-field order.
func RawQueryRow[T any](ctx context.Context, e *Engine, sql string, args ...any) (*T, error) {
	if needsPlaceholderRewrite(e) {
		sql = rewritePlaceholders(sql)
	}

	plan := getScanPlan(reflect.TypeOf((*T)(nil)).Elem())
	if plan.err != nil {
		var zero T
		return nil, fmt.Errorf("drel: RawQueryRow[%T] %w", zero, plan.err)
	}
	row := e.queryRowInternal(ctx, sql, args...)

	rt := reflect.TypeOf((*T)(nil)).Elem()
	v := reflect.New(rt)
	dests := plan.scanDest(v)
	if err := row.Scan(dests...); err != nil {
		return nil, fmt.Errorf("drel: raw query row scan: %w", err)
	}
	return v.Interface().(*T), nil
}
