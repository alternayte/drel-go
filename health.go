package drel

import (
	"context"
	"errors"
	"fmt"

	"github.com/alternayte/drel/internal/driver"
)

// PoolStats is a dialect-neutral snapshot of connection-pool utilisation,
// suitable for a metrics endpoint. AcquiredConns is the number of connections
// currently in use; IdleConns are available; TotalConns is the sum currently
// held; MaxConns is the configured ceiling (0 means driver default).
type PoolStats = driver.PoolStat

// Ping verifies a working connection to the primary database. Wire it into a
// liveness probe (e.g. /healthz). It respects ctx cancellation/deadline.
func (e *Engine) Ping(ctx context.Context) error {
	return e.drv.Ping(ctx)
}

// HealthCheck pings the primary and every registered read replica, returning a
// joined error naming any unreachable endpoint. Wire it into a readiness probe
// (e.g. /readyz). A replica failure does not mask a primary failure: both are
// reported.
func (e *Engine) HealthCheck(ctx context.Context) error {
	var errs []error
	if err := e.drv.Ping(ctx); err != nil {
		errs = append(errs, fmt.Errorf("primary: %w", err))
	}
	for i, r := range e.replicas {
		if err := r.Ping(ctx); err != nil {
			errs = append(errs, fmt.Errorf("replica %d: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

// Stats returns a snapshot of the primary connection pool's utilisation.
func (e *Engine) Stats() PoolStats {
	return e.drv.Stat()
}
