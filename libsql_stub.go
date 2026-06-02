//go:build !libsql

package drel

import (
	"errors"

	"github.com/alternayte/drel/internal/driver"
)

// ErrLibSQLNotBuilt is returned when a libSQL/Turso DSN is used but the binary
// was not compiled with the "libsql" build tag.
var ErrLibSQLNotBuilt = errors.New(
	"drel: libSQL/Turso support not built — rebuild with -tags libsql to enable it")

func newLibSQLDriver(dsn string) (driver.Driver, error) {
	return nil, ErrLibSQLNotBuilt
}
