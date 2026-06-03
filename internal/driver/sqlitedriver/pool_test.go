package sqlitedriver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alternayte/drel/internal/driver"
)

func TestNew_PoolConfigApplied(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "p.db")
	d, err := New(dsn, driver.PoolConfig{MaxConns: 5, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got := d.db.Stats().MaxOpenConnections; got != 5 {
		t.Fatalf("MaxOpenConnections = %d, want 5", got)
	}
}

func TestNew_InMemoryPinnedToOneConn(t *testing.T) {
	// In-memory must stay pinned to one connection regardless of MaxConns.
	d, err := New(":memory:", driver.PoolConfig{MaxConns: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got := d.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("in-memory MaxOpenConnections = %d, want 1", got)
	}
}
