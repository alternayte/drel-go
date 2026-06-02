package driver

import "context"

// IsolationLevel represents a transaction isolation level.
type IsolationLevel int

const (
	IsoDefault IsolationLevel = iota
	IsoReadCommitted
	IsoRepeatableRead
	IsoSerializable
)

// TxOptions configures transaction behaviour.
type TxOptions struct {
	Isolation IsolationLevel
	ReadOnly  bool
}

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

// BatchItem is a single queued query in a pipelined batch.
type BatchItem struct {
	SQL  string
	Args []any
}

// BatchResults yields the result rows for each queued query, in order.
type BatchResults interface {
	// Query returns the rows for the next queued query.
	Query() (Rows, error)
	// Close releases the batch results.
	Close() error
}

// Pipeliner is an optional Driver capability: sending multiple queries in a
// single network round-trip (e.g. the pgx pipeline). Drivers that do not
// implement it fall back to sequential execution.
type Pipeliner interface {
	SendBatch(ctx context.Context, items []BatchItem) (BatchResults, error)
}

// Driver abstracts database access for the drel engine.
type Driver interface {
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	Begin(ctx context.Context) (Tx, error)
	BeginTx(ctx context.Context, opts TxOptions) (Tx, error)
	Close()
}
