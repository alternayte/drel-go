package drel

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
