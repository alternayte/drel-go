package drel

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alternayte/drel/internal/dialect/postgres"
	"github.com/alternayte/drel/internal/driver"
)

// TestN1Detector_EvictsStaleAndCaps proves the counts map does not grow without
// bound: stale entries (window lapsed) are dropped and the map is capped.
func TestN1Detector_EvictsStaleAndCaps(t *testing.T) {
	d := newN1Detector()
	base := time.Unix(0, 0)
	cur := base
	d.now = func() time.Time { return cur }

	// Observe many distinct shapes within one window — must stay capped.
	for i := 0; i < n1MaxShapes*3; i++ {
		d.observe(fmt.Sprintf("SELECT %d", i))
	}
	d.mu.Lock()
	size := len(d.counts)
	d.mu.Unlock()
	if size > n1MaxShapes {
		t.Fatalf("counts map exceeded cap: got %d, want <= %d", size, n1MaxShapes)
	}

	// Advance past the window and observe one shape — stale entries must be gone.
	cur = base.Add(d.window + time.Second)
	d.observe("SELECT fresh")
	d.mu.Lock()
	for k, e := range d.counts {
		if cur.Sub(e.first) > d.window {
			d.mu.Unlock()
			t.Fatalf("stale entry %q was not evicted", k)
		}
	}
	d.mu.Unlock()
}

// TestN1Detector_WarnsAgainAfterWindow proves a re-emerging N+1 warns again
// instead of being silenced forever by a sticky warned flag.
func TestN1Detector_WarnsAgainAfterWindow(t *testing.T) {
	d := newN1Detector()
	cur := time.Unix(0, 0)
	d.now = func() time.Time { return cur }

	warned := 0
	fire := func() {
		for i := 0; i < d.threshold+1; i++ {
			if d.observe("SELECT hot") {
				warned++
			}
		}
	}
	fire()
	if warned != 1 {
		t.Fatalf("first burst: got %d warnings, want 1", warned)
	}

	// Advance past the window so the shape's entry lapses, then burst again.
	cur = cur.Add(d.window + time.Second)
	fire()
	if warned != 2 {
		t.Fatalf("after window: got %d total warnings, want 2 (re-emerging N+1 must warn again)", warned)
	}
}

// explainDeadlineDriver records the deadline of the ctx passed to its EXPLAIN query.
type explainDeadlineDriver struct {
	driver.Driver
	gotDeadline bool
}

func (d *explainDeadlineDriver) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	if _, ok := ctx.Deadline(); ok {
		d.gotDeadline = true
	}
	return d.Driver.Query(ctx, sql, args...)
}

// TestCheckMissingIndex_AppliesTimeout proves the EXPLAIN probe runs under an
// independent deadline rather than inheriting only the (possibly unbounded)
// caller ctx.
func TestCheckMissingIndex_AppliesTimeout(t *testing.T) {
	// Build an engine whose dialect supports Explain (Postgres dialect string),
	// wrapping a no-op driver that records whether the EXPLAIN ctx had a deadline.
	rec := &explainDeadlineDriver{Driver: noopExplainDriver{}}
	e := &Engine{drv: rec, dia: postgres.New()}

	e.checkMissingIndex(context.Background(), "SELECT 1", nil)
	if !rec.gotDeadline {
		t.Fatal("EXPLAIN probe ctx should carry an independent deadline")
	}
}

// noopExplainDriver returns an empty result set for any query.
type noopExplainDriver struct{}

func (noopExplainDriver) QueryRow(context.Context, string, ...any) driver.Row { return nil }
func (noopExplainDriver) Query(context.Context, string, ...any) (driver.Rows, error) {
	return emptyRows{}, nil
}
func (noopExplainDriver) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (noopExplainDriver) Begin(context.Context) (driver.Tx, error)            { return nil, nil }
func (noopExplainDriver) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return nil, nil
}
func (noopExplainDriver) Ping(context.Context) error { return nil }
func (noopExplainDriver) Stat() driver.PoolStat      { return driver.PoolStat{} }
func (noopExplainDriver) Close()                     {}

type emptyRows struct{}

func (emptyRows) Next() bool        { return false }
func (emptyRows) Scan(...any) error { return nil }
func (emptyRows) Close()            {}
func (emptyRows) Err() error        { return nil }
