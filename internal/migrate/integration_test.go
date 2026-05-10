//go:build integration

package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alternayte/drel/internal/driver/pgxdriver"
	"github.com/alternayte/drel/internal/migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupMigrateDB(t *testing.T) *pgxdriver.PgxDriver {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dreltest"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	drv, err := pgxdriver.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { drv.Close() })

	return drv
}

func writeFiles(t *testing.T, dir, version, name, up, down string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, version+"_"+name+".up.sql"), []byte(up), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, version+"_"+name+".down.sql"), []byte(down), 0644))
}

func TestIntegration_Migrate_Up(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);",
		"DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	count, err := runner.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	row := drv.QueryRow(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'users'")
	var c int
	require.NoError(t, row.Scan(&c))
	assert.Equal(t, 1, c)
}

func TestIntegration_Migrate_Up_Idempotent(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY);", "DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	c1, _ := runner.Up(ctx)
	c2, _ := runner.Up(ctx)
	assert.Equal(t, 1, c1)
	assert.Equal(t, 0, c2)
}

func TestIntegration_Migrate_Up_Multiple(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);", "DROP TABLE users;")
	writeFiles(t, dir, "20260510130000", "add_email",
		"ALTER TABLE users ADD COLUMN email TEXT;", "ALTER TABLE users DROP COLUMN email;")

	runner := migrate.NewRunner(drv, dir)
	count, err := runner.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestIntegration_Migrate_Down(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY);", "DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	_, _ = runner.Up(ctx)

	err := runner.Down(ctx)
	require.NoError(t, err)

	row := drv.QueryRow(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'users'")
	var c int
	require.NoError(t, row.Scan(&c))
	assert.Equal(t, 0, c)
}

func TestIntegration_Migrate_Down_OnlyLast(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY);", "DROP TABLE users;")
	writeFiles(t, dir, "20260510130000", "create_posts",
		"CREATE TABLE posts (id SERIAL PRIMARY KEY);", "DROP TABLE posts;")

	runner := migrate.NewRunner(drv, dir)
	_, _ = runner.Up(ctx)
	_ = runner.Down(ctx)

	row := drv.QueryRow(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'users'")
	var usersCount int
	require.NoError(t, row.Scan(&usersCount))
	assert.Equal(t, 1, usersCount)

	row = drv.QueryRow(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'posts'")
	var postsCount int
	require.NoError(t, row.Scan(&postsCount))
	assert.Equal(t, 0, postsCount)
}

func TestIntegration_Migrate_Status(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY);", "DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	_, _ = runner.Up(ctx)

	writeFiles(t, dir, "20260510130000", "create_posts",
		"CREATE TABLE posts (id SERIAL PRIMARY KEY);", "DROP TABLE posts;")

	statuses, err := runner.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.True(t, statuses[0].Applied)
	assert.False(t, statuses[1].Applied)
}

func TestIntegration_Migrate_Lint_DetectsTampering(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY);", "DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	_, _ = runner.Up(ctx)

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "20260510120000_create_users.up.sql"),
		[]byte("CREATE TABLE users (id SERIAL PRIMARY KEY, hacked BOOLEAN);"), 0644))

	issues, err := runner.Lint(ctx)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "20260510120000", issues[0].Version)
}

func TestIntegration_Migrate_Lint_Clean(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY);", "DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	_, _ = runner.Up(ctx)

	issues, err := runner.Lint(ctx)
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestIntegration_Migrate_Roundtrip(t *testing.T) {
	drv := setupMigrateDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFiles(t, dir, "20260510120000", "create_users",
		"CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);", "DROP TABLE users;")

	runner := migrate.NewRunner(drv, dir)
	_, _ = runner.Up(ctx)

	_, _ = drv.Exec(ctx, "INSERT INTO users (name) VALUES ('Alice')")

	_ = runner.Down(ctx)
	_, _ = runner.Up(ctx)

	row := drv.QueryRow(ctx, "SELECT COUNT(*) FROM users")
	var c int
	require.NoError(t, row.Scan(&c))
	assert.Equal(t, 0, c)
}
