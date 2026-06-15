// Package dsn centralises DSN dialect detection, driver opening, and auth-token
// injection so the runtime engine and the migrate CLI share one implementation
// and cannot drift apart.
package dsn

import (
	"context"
	"strings"

	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/libsqldriver"
	"github.com/alternayte/drel/internal/driver/pgxdriver"
	"github.com/alternayte/drel/internal/driver/sqlitedriver"
)

// DetectDialect inspects the DSN and returns "libsql", "sqlite", or "postgres".
// SQLite is recognised by the "file:"/"sqlite://" prefix, ":memory:", a
// ".db"/".sqlite"/".sqlite3" suffix, or — conservatively — a schemeless DSN that
// looks like a filesystem path rather than a Postgres host/host:port/keyword DSN.
// "postgres://"/"postgresql://", host:port, user@host and "key=value" DSNs map to
// "postgres". Use WithDialect/WithDriver to override detection.
//
//   - "libsql": libsql:// wss:// ws:// http:// https:// (Turso / local sqld).
//   - "sqlite": file: prefix, sqlite:// prefix, ":memory:", a path ending in
//     ".db"/".sqlite"/".sqlite3", or a schemeless path that is not a Postgres target.
//   - "postgres": postgres://, postgresql://, host:port, user@host, or libpq key=value.
func DetectDialect(d string) string {
	if strings.HasPrefix(d, "libsql://") ||
		strings.HasPrefix(d, "wss://") ||
		strings.HasPrefix(d, "ws://") ||
		strings.HasPrefix(d, "http://") ||
		strings.HasPrefix(d, "https://") {
		return "libsql"
	}
	if strings.HasPrefix(d, "postgres://") || strings.HasPrefix(d, "postgresql://") {
		return "postgres"
	}
	if strings.HasPrefix(d, "file:") ||
		strings.HasPrefix(d, "sqlite://") ||
		d == ":memory:" ||
		strings.HasSuffix(d, ".db") ||
		strings.HasSuffix(d, ".sqlite") ||
		strings.HasSuffix(d, ".sqlite3") {
		return "sqlite"
	}
	// No recognised URL scheme. Distinguish a Postgres connection target from a
	// SQLite file path conservatively: a Postgres DSN has a "host:port" shape, a
	// "user@host" shape, or libpq "key=value" pairs. Anything else (a relative or
	// absolute filesystem path) is treated as a SQLite file.
	if looksLikePostgresTarget(d) {
		return "postgres"
	}
	return "sqlite"
}

// looksLikePostgresTarget reports whether a schemeless DSN resembles a Postgres
// connection target rather than a SQLite file path. It matches libpq keyword DSNs
// ("host=... dbname=..."), "user@host" forms, and "host:port" forms where the
// part after the final ":" is all digits. Filesystem-looking paths (containing
// "/", starting with "." or "/", or with no ":"/"@"/"=" markers) are not Postgres.
func looksLikePostgresTarget(d string) bool {
	if d == "" {
		return false
	}
	// libpq keyword/value DSN, e.g. "host=localhost dbname=app".
	if strings.Contains(d, "=") && !strings.ContainsAny(d, "/\\") {
		return true
	}
	// "user@host" form (no path separators).
	if strings.Contains(d, "@") && !strings.ContainsAny(d, "/\\") {
		return true
	}
	// "host:port" form: the suffix after the last ":" is all digits, and the DSN
	// has no path separators (a Windows path like "C:\db" or a relative "a:b/c"
	// would have separators and is treated as a file).
	if i := strings.LastIndex(d, ":"); i >= 0 && !strings.ContainsAny(d, "/\\") {
		port := d[i+1:]
		if port != "" && isAllDigits(port) {
			return true
		}
	}
	return false
}

// isAllDigits reports whether s is non-empty and contains only ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ApplyAuthToken appends an authToken query parameter to a libSQL DSN when a
// token is supplied and the DSN does not already carry one. It is a no-op for an
// empty token or a DSN that already has authToken=.
func ApplyAuthToken(d, token string) string {
	if token == "" || strings.Contains(d, "authToken=") {
		return d
	}
	sep := "?"
	if strings.Contains(d, "?") {
		sep = "&"
	}
	return d + sep + "authToken=" + token
}

// OpenDriver opens a driver for the DSN using DetectDialect. For a libsql DSN the
// auth token (if any) is injected via ApplyAuthToken. The token is ignored for
// non-libsql dialects.
func OpenDriver(ctx context.Context, d, authToken string) (driver.Driver, error) {
	switch DetectDialect(d) {
	case "libsql":
		return libsqldriver.New(ApplyAuthToken(d, authToken))
	case "sqlite":
		return sqlitedriver.New(d)
	default:
		return pgxdriver.New(ctx, d)
	}
}
