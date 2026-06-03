package drel

import "github.com/alternayte/drel/internal/dberr"

// Dialect-neutral database error sentinels. Match them with errors.Is on errors
// returned by SaveChanges, bulk operations, raw Exec/Query, etc. The original
// driver error (e.g. *pgconn.PgError) is still reachable via errors.As.
//
//	if err := uow.SaveChanges(ctx); errors.Is(err, drel.ErrUniqueViolation) {
//	    return conflict()
//	}
var (
	// ErrUniqueViolation is returned when an INSERT/UPDATE violates a unique or
	// primary-key constraint.
	ErrUniqueViolation = dberr.ErrUniqueViolation
	// ErrForeignKeyViolation is returned when a foreign-key constraint fails.
	ErrForeignKeyViolation = dberr.ErrForeignKeyViolation
	// ErrNotNullViolation is returned when a NOT NULL column receives NULL.
	ErrNotNullViolation = dberr.ErrNotNullViolation
	// ErrCheckViolation is returned when a CHECK constraint fails.
	ErrCheckViolation = dberr.ErrCheckViolation
	// ErrSerializationFailure is returned for serialization failures / deadlocks
	// under high isolation; the transaction should be retried.
	ErrSerializationFailure = dberr.ErrSerializationFailure
)

// classifyRow wraps a Row so Scan errors are classified into the sentinels above.
type classifyRow struct{ row Row }

func (r classifyRow) Scan(dest ...any) error {
	return dberr.Classify(r.row.Scan(dest...))
}
