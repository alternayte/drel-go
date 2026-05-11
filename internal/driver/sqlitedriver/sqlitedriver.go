package sqlitedriver

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alternayte/drel/internal/driver"
	_ "modernc.org/sqlite"
)

// SQLiteDriver implements driver.Driver using database/sql with the modernc.org/sqlite pure-Go driver.
type SQLiteDriver struct {
	db *sql.DB
}

// New opens a SQLite database at the given DSN and enables WAL mode.
// No context is required for connection — SQLite is an embedded database.
func New(dsn string) (*SQLiteDriver, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlitedriver: open: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlitedriver: enable WAL: %w", err)
	}
	return &SQLiteDriver{db: db}, nil
}

// QueryRow executes a query that returns at most one row.
func (d *SQLiteDriver) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

// Query executes a query that returns multiple rows.
func (d *SQLiteDriver) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &sqliteRows{rows: rows}, nil
}

// Exec executes a statement and returns the number of affected rows.
func (d *SQLiteDriver) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Begin starts a new database transaction with default options.
func (d *SQLiteDriver) Begin(ctx context.Context) (driver.Tx, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx}, nil
}

// BeginTx starts a new database transaction with the given options.
// SQLite only supports SERIALIZABLE isolation, so the isolation level is ignored.
// Only the ReadOnly flag is forwarded.
func (d *SQLiteDriver) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	sqlOpts := &sql.TxOptions{
		ReadOnly: opts.ReadOnly,
		// SQLite supports only SERIALIZABLE; ignore the requested isolation level.
	}
	tx, err := d.db.BeginTx(ctx, sqlOpts)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx}, nil
}

// Close closes the underlying database connection.
func (d *SQLiteDriver) Close() {
	d.db.Close()
}

// sqliteRows wraps *sql.Rows to satisfy driver.Rows.
type sqliteRows struct {
	rows *sql.Rows
}

func (r *sqliteRows) Next() bool            { return r.rows.Next() }
func (r *sqliteRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *sqliteRows) Close()                { r.rows.Close() }
func (r *sqliteRows) Err() error            { return r.rows.Err() }
