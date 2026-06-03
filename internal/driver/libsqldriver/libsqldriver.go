// Package libsqldriver implements driver.Driver for libSQL / Turso using the
// database/sql libsql driver.
package libsqldriver

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alternayte/drel/internal/driver"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// LibSQLDriver implements driver.Driver over a libSQL/Turso connection.
type LibSQLDriver struct {
	db *sql.DB
}

// New opens a libSQL database at the given DSN (e.g. "libsql://name.turso.io?authToken=...").
func New(dsn string) (*LibSQLDriver, error) {
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("libsqldriver: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("libsqldriver: open: %w", err)
	}
	return &LibSQLDriver{db: db}, nil
}

func (d *LibSQLDriver) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

func (d *LibSQLDriver) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &libsqlRows{rows: rows}, nil
}

func (d *LibSQLDriver) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *LibSQLDriver) Begin(ctx context.Context) (driver.Tx, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &libsqlTx{tx: tx}, nil
}

// BeginTx starts a transaction. libSQL supports only SERIALIZABLE isolation, so
// the requested level is ignored; the ReadOnly flag is forwarded.
func (d *LibSQLDriver) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: opts.ReadOnly})
	if err != nil {
		return nil, err
	}
	return &libsqlTx{tx: tx}, nil
}

func (d *LibSQLDriver) Close() { d.db.Close() }

type libsqlRows struct{ rows *sql.Rows }

func (r *libsqlRows) Next() bool             { return r.rows.Next() }
func (r *libsqlRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *libsqlRows) Close()                 { r.rows.Close() }
func (r *libsqlRows) Err() error             { return r.rows.Err() }

type libsqlTx struct{ tx *sql.Tx }

func (t *libsqlTx) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}

func (t *libsqlTx) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &libsqlRows{rows: rows}, nil
}

func (t *libsqlTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (t *libsqlTx) Commit(ctx context.Context) error   { return t.tx.Commit() }
func (t *libsqlTx) Rollback(ctx context.Context) error { return t.tx.Rollback() }
