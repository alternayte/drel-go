package dberr

import (
	"errors"
	"testing"
)

// TestClassify_LibSQLMessageMatch pins the libSQL/Turso string-match contract:
// libsql-client-go returns raw database/sql string errors, not typed
// *sqlite.Error, so message matching is the sole classification mechanism for
// that driver. This is the path that previously had zero tests.
func TestClassify_LibSQLMessageMatch(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want error
	}{
		{"unique", "UNIQUE constraint failed: users.email", ErrUniqueViolation},
		{"primary key", "PRIMARY KEY constraint failed: users.id", ErrUniqueViolation},
		{"foreign key", "FOREIGN KEY constraint failed", ErrForeignKeyViolation},
		{"not null", "NOT NULL constraint failed: users.name", ErrNotNullViolation},
		{"check", "CHECK constraint failed: age_non_negative", ErrCheckViolation},
		{"database is locked", "database is locked", ErrSerializationFailure},
		{"database table is locked", "database table is locked: users", ErrSerializationFailure},
		{"sqlite busy token", "SQLITE_BUSY: database is busy", ErrSerializationFailure},
		// libsql/sqld surfaces conflicts with this phrasing under WAL.
		{"busy snapshot phrase", "cannot start a transaction within a transaction (SQLITE_BUSY_SNAPSHOT)", ErrSerializationFailure},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(errors.New(c.msg))
			if !errors.Is(got, c.want) {
				t.Fatalf("Classify(%q): want errors.Is(_, %v), got %v", c.msg, c.want, got)
			}
		})
	}
}

// TestClassify_NoFalsePositive confirms unrelated errors stay unclassified.
func TestClassify_NoFalsePositive(t *testing.T) {
	got := Classify(errors.New("syntax error near SELEC"))
	for _, k := range []error{ErrUniqueViolation, ErrForeignKeyViolation, ErrNotNullViolation, ErrCheckViolation, ErrSerializationFailure} {
		if errors.Is(got, k) {
			t.Fatalf("unrelated error wrongly classified as %v", k)
		}
	}
}

// TestClassify_NilPassthrough keeps the nil-in/nil-out contract.
func TestClassify_NilPassthrough(t *testing.T) {
	if Classify(nil) != nil {
		t.Fatal("Classify(nil) must be nil")
	}
}
