package sqlitedriver_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/sqlitedriver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDriver(t *testing.T) *sqlitedriver.SQLiteDriver {
	t.Helper()
	d, err := sqlitedriver.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	return d
}

func TestNew(t *testing.T) {
	d := newTestDriver(t)
	assert.NotNil(t, d)
}

func TestExec_CreateTable(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	n, err := d.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestExec_Insert(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)

	n, err := d.Exec(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 1, "Alice")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestQueryRow(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = d.Exec(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 42, "Bob")
	require.NoError(t, err)

	row := d.QueryRow(ctx, `SELECT id, name FROM users WHERE id = ?`, 42)
	var id int
	var name string
	require.NoError(t, row.Scan(&id, &name))
	assert.Equal(t, 42, id)
	assert.Equal(t, "Bob", name)
}

func TestQuery_MultipleRows(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = d.Exec(ctx, `INSERT INTO users (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Carol')`)
	require.NoError(t, err)

	rows, err := d.Query(ctx, `SELECT id, name FROM users ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()

	type user struct {
		id   int
		name string
	}
	var results []user
	for rows.Next() {
		var u user
		require.NoError(t, rows.Scan(&u.id, &u.name))
		results = append(results, u)
	}
	require.NoError(t, rows.Err())

	require.Len(t, results, 3)
	assert.Equal(t, user{1, "Alice"}, results[0])
	assert.Equal(t, user{2, "Bob"}, results[1])
	assert.Equal(t, user{3, "Carol"}, results[2])
}

func TestTransaction_Commit(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, val TEXT NOT NULL)`)
	require.NoError(t, err)

	tx, err := d.Begin(ctx)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `INSERT INTO items (id, val) VALUES (?, ?)`, 1, "committed")
	require.NoError(t, err)

	require.NoError(t, tx.Commit(ctx))

	// Verify the row is visible outside the transaction.
	row := d.QueryRow(ctx, `SELECT val FROM items WHERE id = ?`, 1)
	var val string
	require.NoError(t, row.Scan(&val))
	assert.Equal(t, "committed", val)
}

func TestTransaction_Rollback(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, val TEXT NOT NULL)`)
	require.NoError(t, err)

	tx, err := d.Begin(ctx)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `INSERT INTO items (id, val) VALUES (?, ?)`, 1, "rolled-back")
	require.NoError(t, err)

	require.NoError(t, tx.Rollback(ctx))

	// Verify the row is NOT visible after rollback.
	row := d.QueryRow(ctx, `SELECT COUNT(*) FROM items WHERE id = ?`, 1)
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count)
}

func TestBeginTx_ReadOnly(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, val TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = d.Exec(ctx, `INSERT INTO items (id, val) VALUES (1, 'existing')`)
	require.NoError(t, err)

	tx, err := d.BeginTx(ctx, driver.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `SELECT val FROM items WHERE id = ?`, 1)
	var val string
	require.NoError(t, row.Scan(&val))
	assert.Equal(t, "existing", val)
}

func TestTransaction_QueryAndQueryRow(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	_, err := d.Exec(ctx, `CREATE TABLE nums (n INTEGER NOT NULL)`)
	require.NoError(t, err)
	_, err = d.Exec(ctx, `INSERT INTO nums VALUES (10), (20), (30)`)
	require.NoError(t, err)

	tx, err := d.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck

	// QueryRow inside transaction.
	row := tx.QueryRow(ctx, `SELECT SUM(n) FROM nums`)
	var sum int
	require.NoError(t, row.Scan(&sum))
	assert.Equal(t, 60, sum)

	// Query inside transaction.
	rows, err := tx.Query(ctx, `SELECT n FROM nums ORDER BY n`)
	require.NoError(t, err)
	defer rows.Close()

	var nums []int
	for rows.Next() {
		var n int
		require.NoError(t, rows.Scan(&n))
		nums = append(nums, n)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []int{10, 20, 30}, nums)
}
