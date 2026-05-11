package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rawPerson struct {
	Name string `db:"name"`
	Age  int    `db:"age"`
}

func setupRawQueryEngine(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `
		CREATE TABLE people (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			age  INTEGER NOT NULL
		)
	`)
	require.NoError(t, err)

	_, err = engine.Exec(ctx, "INSERT INTO people (name, age) VALUES (?, ?)", "alice", 30)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO people (name, age) VALUES (?, ?)", "bob", 25)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO people (name, age) VALUES (?, ?)", "charlie", 35)
	require.NoError(t, err)

	return engine
}

func TestRawQuery_MultipleRows(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	results, err := drel.RawQuery[rawPerson](ctx, engine,
		"SELECT name, age FROM people WHERE age > $1 ORDER BY name", 20)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.Equal(t, "alice", results[0].Name)
	assert.Equal(t, 30, results[0].Age)
	assert.Equal(t, "bob", results[1].Name)
	assert.Equal(t, 25, results[1].Age)
	assert.Equal(t, "charlie", results[2].Name)
	assert.Equal(t, 35, results[2].Age)
}

func TestRawQuery_WithFilter(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	results, err := drel.RawQuery[rawPerson](ctx, engine,
		"SELECT name, age FROM people WHERE age >= $1 ORDER BY age", 30)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "alice", results[0].Name)
	assert.Equal(t, "charlie", results[1].Name)
}

func TestRawQuery_NoResults(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	results, err := drel.RawQuery[rawPerson](ctx, engine,
		"SELECT name, age FROM people WHERE age > $1", 100)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestRawQuery_MultiplePlaceholders(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	results, err := drel.RawQuery[rawPerson](ctx, engine,
		"SELECT name, age FROM people WHERE age > $1 AND age < $2 ORDER BY name", 24, 32)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "alice", results[0].Name)
	assert.Equal(t, "bob", results[1].Name)
}

func TestRawQueryRow_SingleRow(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	result, err := drel.RawQueryRow[rawPerson](ctx, engine,
		"SELECT name, age FROM people WHERE name = $1", "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", result.Name)
	assert.Equal(t, 30, result.Age)
}

func TestRawQueryRow_NotFound(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	_, err := drel.RawQueryRow[rawPerson](ctx, engine,
		"SELECT name, age FROM people WHERE name = $1", "nonexistent")
	assert.Error(t, err)
}

type rawCount struct {
	Total int `db:"total"`
}

func TestRawQueryRow_Aggregate(t *testing.T) {
	engine := setupRawQueryEngine(t)
	ctx := context.Background()

	result, err := drel.RawQueryRow[rawCount](ctx, engine,
		"SELECT COUNT(*) AS total FROM people WHERE age > $1", 20)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
}
