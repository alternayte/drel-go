package drel

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchAfterCommit_DetachesCtxAndRecovers(t *testing.T) {
	var mu sync.Mutex
	var sinkErrs []error
	engine, err := NewEngine(":memory:", WithAfterCommitErrorSink(func(ctx context.Context, e error) {
		mu.Lock()
		defer mu.Unlock()
		sinkErrs = append(sinkErrs, e)
	}))
	require.NoError(t, err)
	defer engine.Close()

	var ranSecond bool
	var sawCancelled bool

	// First hook panics; it must not prevent the second from running.
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		panic("boom in hook 1")
	})
	// Second hook checks the ctx is NOT cancelled even though we pass a
	// cancelled ctx into the dispatch.
	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		mu.Lock()
		defer mu.Unlock()
		ranSecond = true
		sawCancelled = ctx.Err() != nil
	})

	// Build a cancelled ctx that still carries values.
	type ctxKey struct{}
	parent := context.WithValue(context.Background(), ctxKey{}, "v")
	cctx, cancel := context.WithCancel(parent)
	cancel() // cancel before dispatch

	// dispatchAfterCommit must not panic out, must run both hooks, must detach
	// cancellation, and must report the panic to the sink.
	require.NotPanics(t, func() {
		engine.dispatchAfterCommit(cctx, []any{"e1"})
	})

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, ranSecond, "second hook must run despite first hook panicking")
	assert.False(t, sawCancelled, "after-commit ctx must be detached from cancellation")
	require.Len(t, sinkErrs, 1, "the panicking hook must be reported to the sink")
	assert.Contains(t, sinkErrs[0].Error(), "boom in hook 1")
}

func TestDispatchAfterCommit_NoSinkDoesNotCrash(t *testing.T) {
	engine, err := NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()

	engine.OnAfterCommit(func(ctx context.Context, events []any) {
		panic("boom, no sink configured")
	})
	require.NotPanics(t, func() {
		engine.dispatchAfterCommit(context.Background(), nil)
	})
}

func TestWithAfterCommitErrorSink_Configures(t *testing.T) {
	var mu sync.Mutex
	var got []error
	sink := func(ctx context.Context, err error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, err)
	}

	engine, err := NewEngine(":memory:", WithAfterCommitErrorSink(sink))
	require.NoError(t, err)
	defer engine.Close()

	// The sink must be stored on the engine.
	require.NotNil(t, engine.afterCommitSink)

	// Invoking it directly routes through to the configured sink.
	engine.afterCommitSink(context.Background(), assert.AnError)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	assert.ErrorIs(t, got[0], assert.AnError)
}
