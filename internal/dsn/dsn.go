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
//
//   - "libsql": libsql:// wss:// ws:// http:// https:// (Turso / local sqld).
//   - "sqlite": file: prefix, sqlite:// prefix, ":memory:", or a path ending in
//     ".db".
//   - "postgres": everything else (postgres://, postgresql://, libpq key=value).
func DetectDialect(d string) string {
	if strings.HasPrefix(d, "libsql://") ||
		strings.HasPrefix(d, "wss://") ||
		strings.HasPrefix(d, "ws://") ||
		strings.HasPrefix(d, "http://") ||
		strings.HasPrefix(d, "https://") {
		return "libsql"
	}
	if strings.HasPrefix(d, "file:") ||
		strings.HasPrefix(d, "sqlite://") ||
		d == ":memory:" ||
		strings.HasSuffix(d, ".db") {
		return "sqlite"
	}
	return "postgres"
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
