package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMigrationDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510120000_create_users.up.sql"), []byte("CREATE TABLE users();"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510120000_create_users.down.sql"), []byte("DROP TABLE users;"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510130000_add_posts.up.sql"), []byte("CREATE TABLE posts();"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510130000_add_posts.down.sql"), []byte("DROP TABLE posts;"), 0644))

	migrations, err := ParseMigrationDir(dir)
	require.NoError(t, err)
	require.Len(t, migrations, 2)

	assert.Equal(t, "20260510120000", migrations[0].Version)
	assert.Equal(t, "create_users", migrations[0].Name)
	assert.Equal(t, "CREATE TABLE users();", migrations[0].UpSQL)
	assert.Equal(t, "DROP TABLE users;", migrations[0].DownSQL)
	assert.Equal(t, "20260510130000", migrations[1].Version)
}

func TestParseMigrationDir_NonExistent(t *testing.T) {
	migrations, err := ParseMigrationDir("/nonexistent")
	require.NoError(t, err)
	assert.Empty(t, migrations)
}

func TestParseMigrationDir_Sorted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510130000_b.up.sql"), []byte("B"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510130000_b.down.sql"), []byte("B"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510120000_a.up.sql"), []byte("A"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260510120000_a.down.sql"), []byte("A"), 0644))

	migrations, err := ParseMigrationDir(dir)
	require.NoError(t, err)
	assert.Equal(t, "20260510120000", migrations[0].Version)
	assert.Equal(t, "20260510130000", migrations[1].Version)
}

func TestWriteMigration(t *testing.T) {
	dir := t.TempDir()

	version, err := WriteMigration(dir, "create_users", "CREATE TABLE users();", "DROP TABLE users;")
	require.NoError(t, err)
	assert.Len(t, version, 14)

	up, _ := os.ReadFile(filepath.Join(dir, version+"_create_users.up.sql"))
	assert.Equal(t, "CREATE TABLE users();", string(up))

	down, _ := os.ReadFile(filepath.Join(dir, version+"_create_users.down.sql"))
	assert.Equal(t, "DROP TABLE users;", string(down))
}

func TestNewRunner_Dialect(t *testing.T) {
	r := NewRunner(nil, "/tmp/migrations", "postgres")
	assert.Equal(t, "postgres", r.Dialect())

	r2 := NewRunner(nil, "/tmp/migrations", "sqlite")
	assert.Equal(t, "sqlite", r2.Dialect())
}

func TestChecksumContent(t *testing.T) {
	cs1 := ChecksumContent("CREATE TABLE users();")
	cs2 := ChecksumContent("CREATE TABLE users();")
	cs3 := ChecksumContent("CREATE TABLE posts();")

	assert.Equal(t, cs1, cs2)
	assert.NotEqual(t, cs1, cs3)
	assert.Len(t, cs1, 64)
}
