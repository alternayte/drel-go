package driver

import "context"

// Row represents a single database row that can scan values into destinations.
type Row interface {
	Scan(dest ...any) error
}

// Rows represents a set of database rows from a query result.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// Tx represents a database transaction.
type Tx interface {
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Driver abstracts database access for the drel engine.
type Driver interface {
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	Begin(ctx context.Context) (Tx, error)
	Close()
}
