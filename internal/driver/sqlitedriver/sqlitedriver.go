package sqlitedriver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/alternayte/drel/internal/driver"
	_ "modernc.org/sqlite"
)

// isInMemory reports whether a DSN refers to a transient in-memory database.
// Such databases live in a single connection, so the pool must be pinned to one
// connection or each pooled connection would see an independent, empty database.
func isInMemory(dsn string) bool {
	return dsn == ":memory:" ||
		strings.Contains(dsn, ":memory:") ||
		strings.Contains(dsn, "mode=memory")
}

// withPragmas appends modernc.org/sqlite `_pragma` query parameters to a DSN so
// they apply to every pooled connection (a plain `PRAGMA` Exec configures only
// the single connection it runs on). File-backed databases additionally get WAL
// journaling; in-memory databases do not support WAL.
func withPragmas(dsn string, inMemory bool) string {
	pragmas := []string{"_pragma=busy_timeout(5000)", "_pragma=foreign_keys(1)"}
	if !inMemory {
		pragmas = append(pragmas, "_pragma=journal_mode(WAL)")
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + strings.Join(pragmas, "&")
}

// SQLiteDriver implements driver.Driver using database/sql with the modernc.org/sqlite pure-Go driver.
type SQLiteDriver struct {
	db *sql.DB
}

// New opens a SQLite database at the given DSN and enables WAL mode.
// No context is required for connection — SQLite is an embedded database.
func New(dsn string) (*SQLiteDriver, error) {
	inMemory := isInMemory(dsn)
	db, err := sql.Open("sqlite", withPragmas(dsn, inMemory))
	if err != nil {
		return nil, fmt.Errorf("sqlitedriver: open: %w", err)
	}

	if inMemory {
		// A pooled second connection to :memory: would open a separate empty
		// database. Pin to one connection so all work shares the same DB.
		db.SetMaxOpenConns(1)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlitedriver: open: %w", err)
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

func (r *sqliteRows) Next() bool             { return r.rows.Next() }
func (r *sqliteRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *sqliteRows) Close()                 { r.rows.Close() }
func (r *sqliteRows) Err() error             { return r.rows.Err() }
