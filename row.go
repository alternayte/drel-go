package drel

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
