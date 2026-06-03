package drel

import (
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/libsqldriver"
)

// newLibSQLDriver opens a libSQL/Turso driver. libSQL works out of the box for
// libsql:// / https:// / wss:// (etc.) DSNs — no build tag required.
func newLibSQLDriver(dsn string) (driver.Driver, error) {
	return libsqldriver.New(dsn)
}
