package drel_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/require"
)

// TestSerializationFailure_SQLiteBusy provokes a real SQLITE_BUSY by holding a
// write lock with busy_timeout(0) while a second connection attempts to write,
// then asserts the resulting error is classified as ErrSerializationFailure.
// This exercises the typed *sqlite.Error path that Task 1 added (a typed
// *sqlite.Error cannot be constructed directly in a unit test).
func TestSerializationFailure_SQLiteBusy(t *testing.T) {
	dir := t.TempDir()
	// busy_timeout(0) makes a contended write fail immediately instead of waiting.
	dsn := "file:" + filepath.Join(dir, "busy.db") + "?_pragma=busy_timeout(0)&_pragma=journal_mode(WAL)"

	holder, err := drel.NewEngine(dsn)
	require.NoError(t, err)
	defer holder.Close()
	contender, err := drel.NewEngine(dsn)
	require.NoError(t, err)
	defer contender.Close()

	ctx := context.Background()
	_, err = holder.Exec(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY, v INTEGER)`)
	require.NoError(t, err)
	_, err = holder.Exec(ctx, `INSERT INTO t (id, v) VALUES (1, 0)`)
	require.NoError(t, err)

	// Hold an exclusive write lock on the first engine inside an open tx.
	var gotBusy error
	var wg sync.WaitGroup
	release := make(chan struct{})
	locked := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = holder.Transaction(ctx, func(tx *drel.Tx) error {
			// Acquire the write lock.
			if _, e := tx.Exec(ctx, `UPDATE t SET v = v + 1 WHERE id = 1`); e != nil {
				return e
			}
			close(locked)
			<-release // keep the lock held while the contender tries to write
			return nil
		})
	}()

	<-locked
	// Second connection's write should fail immediately with SQLITE_BUSY.
	for i := 0; i < 50 && gotBusy == nil; i++ {
		_, gotBusy = contender.Exec(ctx, `UPDATE t SET v = v + 1 WHERE id = 1`)
	}
	close(release)
	wg.Wait()

	require.Error(t, gotBusy, "contended write should fail with a busy/locked error")
	require.True(t, errors.Is(gotBusy, drel.ErrSerializationFailure),
		"busy write must classify as ErrSerializationFailure, got %v", gotBusy)
}
