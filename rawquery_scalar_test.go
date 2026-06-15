package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawQuery_ScalarColumn(t *testing.T) {
	engine := setupRawQueryEngine(t) // defined in rawquery_test.go
	ctx := context.Background()

	names, err := drel.RawQuery[string](ctx, engine,
		"SELECT name FROM people ORDER BY name")
	require.NoError(t, err)
	require.Equal(t, []*string{strptr("alice"), strptr("bob"), strptr("charlie")}, names)
}

func TestRawQueryRow_ScalarCount(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	total, err := drel.RawQueryRow[int](ctx, engine,
		"SELECT COUNT(*) FROM people WHERE age > $1", 20)
	require.NoError(t, err)
	require.Equal(t, 3, *total)
}

func TestRawQuery_UnsupportedType_ReturnsError(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	_, err := drel.RawQuery[map[string]any](ctx, engine, "SELECT name FROM people")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drel: RawQuery")
	assert.Contains(t, err.Error(), "map")
}

func strptr(s string) *string { return &s }
