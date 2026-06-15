package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_PingHealthCheckStats(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()

	require.NoError(t, engine.Ping(ctx))
	require.NoError(t, engine.HealthCheck(ctx))

	st := engine.Stats()
	// In-memory SQLite pins the pool to a single connection.
	assert.Equal(t, int32(1), st.MaxConns)
	assert.GreaterOrEqual(t, st.TotalConns, int32(0))
}
