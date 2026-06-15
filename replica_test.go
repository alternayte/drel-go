package drel

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/driver"
)

// recDriver is a no-op driver.Driver that records which queries/execs it served.
type recDriver struct {
	name  string
	calls *[]string
}

func (d *recDriver) record(kind string) { *d.calls = append(*d.calls, d.name+":"+kind) }

func (d *recDriver) QueryRow(ctx context.Context, sql string, args ...any) driver.Row {
	d.record("queryrow")
	return nil
}
func (d *recDriver) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	d.record("query")
	return nil, nil
}
func (d *recDriver) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	d.record("exec")
	return 0, nil
}
func (d *recDriver) Begin(ctx context.Context) (driver.Tx, error) { d.record("begin"); return nil, nil }
func (d *recDriver) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	d.record("begintx")
	return nil, nil
}
func (d *recDriver) Close() {}
func (d *recDriver) Ping(ctx context.Context) error { d.record("ping"); return nil }
func (d *recDriver) Stat() driver.PoolStat          { d.record("stat"); return driver.PoolStat{} }

func TestReadDriver_RoundRobinAndPrimary(t *testing.T) {
	var calls []string
	primary := &recDriver{name: "primary", calls: &calls}
	r1 := &recDriver{name: "r1", calls: &calls}
	r2 := &recDriver{name: "r2", calls: &calls}

	e := &Engine{drv: primary, replicas: []driver.Driver{r1, r2}}

	pick := func() string {
		d := e.readDriver(false).(*recDriver)
		return d.name
	}
	// Round-robin across replicas only.
	got := []string{pick(), pick(), pick(), pick()}
	want := []string{"r1", "r2", "r1", "r2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("read %d: got %s want %s (seq %v)", i, got[i], want[i], got)
		}
	}

	// primary=true forces the primary.
	if e.readDriver(true).(*recDriver).name != "primary" {
		t.Fatal("readDriver(true) should return the primary")
	}

	// With no replicas, reads use the primary.
	e2 := &Engine{drv: primary}
	if e2.readDriver(false).(*recDriver).name != "primary" {
		t.Fatal("readDriver(false) with no replicas should return the primary")
	}
}

func TestReadRouting_WritesAndTxUsePrimary(t *testing.T) {
	var calls []string
	primary := &recDriver{name: "primary", calls: &calls}
	r1 := &recDriver{name: "r1", calls: &calls}
	e := &Engine{drv: primary, replicas: []driver.Driver{r1}}
	ctx := context.Background()

	_, _ = e.queryInternal(ctx, "SELECT 1")      // read → replica
	_, _ = e.execInternal(ctx, "UPDATE x SET y") // write → primary
	_, _ = e.drv.Begin(ctx)                      // tx begin is always on primary by construction

	// First call should be the replica; the write must be on the primary.
	if calls[0] != "r1:query" {
		t.Fatalf("read should hit replica, got %v", calls)
	}
	foundPrimaryExec := false
	for _, c := range calls {
		if c == "primary:exec" {
			foundPrimaryExec = true
		}
		if c == "r1:exec" {
			t.Fatalf("write must never hit a replica: %v", calls)
		}
	}
	if !foundPrimaryExec {
		t.Fatalf("write should hit primary: %v", calls)
	}
}
