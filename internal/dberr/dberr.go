// Package dberr classifies driver-specific database errors into a small set of
// dialect-neutral sentinel errors that callers can match with errors.Is,
// without importing pgconn or the sqlite driver themselves.
package dberr

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	sqlite "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

// Sentinel errors for common, actionable database conditions. They are matched
// with errors.Is; the original driver error remains available via errors.As /
// Unwrap.
var (
	ErrUniqueViolation      = errors.New("drel: unique constraint violation")
	ErrForeignKeyViolation  = errors.New("drel: foreign key constraint violation")
	ErrNotNullViolation     = errors.New("drel: not-null constraint violation")
	ErrCheckViolation       = errors.New("drel: check constraint violation")
	ErrSerializationFailure = errors.New("drel: serialization failure (retry the transaction)")
)

// classified wraps an original driver error and reports a sentinel kind via Is,
// while still unwrapping to the original (so errors.As to *pgconn.PgError etc.
// keeps working).
type classified struct {
	kind error
	err  error
}

func (c *classified) Error() string        { return c.err.Error() }
func (c *classified) Unwrap() error        { return c.err }
func (c *classified) Is(target error) bool { return target == c.kind }

// Classify returns err annotated with a sentinel kind when it represents a
// known constraint or serialization condition, or err unchanged otherwise.
// nil in, nil out.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	if kind := kindOf(err); kind != nil {
		return &classified{kind: kind, err: err}
	}
	return err
}

func kindOf(err error) error {
	// Postgres (pgx): SQLSTATE codes.
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505": // unique_violation
			return ErrUniqueViolation
		case "23503": // foreign_key_violation
			return ErrForeignKeyViolation
		case "23502": // not_null_violation
			return ErrNotNullViolation
		case "23514": // check_violation
			return ErrCheckViolation
		case "40001", "40P01": // serialization_failure, deadlock_detected
			return ErrSerializationFailure
		}
		return nil
	}

	// SQLite (modernc): extended result codes.
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqlitelib.SQLITE_CONSTRAINT_UNIQUE, sqlitelib.SQLITE_CONSTRAINT_PRIMARYKEY:
			return ErrUniqueViolation
		case sqlitelib.SQLITE_CONSTRAINT_FOREIGNKEY:
			return ErrForeignKeyViolation
		case sqlitelib.SQLITE_CONSTRAINT_NOTNULL:
			return ErrNotNullViolation
		case sqlitelib.SQLITE_CONSTRAINT_CHECK:
			return ErrCheckViolation
		}
		return nil
	}

	// libSQL/Turso and any other SQLite-compatible driver: match the standard
	// SQLite constraint messages.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed"),
		strings.Contains(msg, "PRIMARY KEY constraint failed"):
		return ErrUniqueViolation
	case strings.Contains(msg, "FOREIGN KEY constraint failed"):
		return ErrForeignKeyViolation
	case strings.Contains(msg, "NOT NULL constraint failed"):
		return ErrNotNullViolation
	case strings.Contains(msg, "CHECK constraint failed"):
		return ErrCheckViolation
	}
	return nil
}
