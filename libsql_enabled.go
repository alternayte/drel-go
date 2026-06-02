//go:build libsql

package drel

import (
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/libsqldriver"
)

// newLibSQLDriver opens a libSQL/Turso driver. Compiled only with the "libsql"
// build tag so the libsql client stays out of builds that don't need it.
func newLibSQLDriver(dsn string) (driver.Driver, error) {
	return libsqldriver.New(dsn)
}
