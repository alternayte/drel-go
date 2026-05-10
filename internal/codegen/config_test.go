package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drel.yaml")
	os.WriteFile(path, []byte("packages:\n  - ./features/users\n  - ./features/posts\noutput:\n  db: ./db/drel_gen.go\ndialect: postgres\n"), 0644)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"./features/users", "./features/posts"}, cfg.Packages)
	assert.Equal(t, "./db/drel_gen.go", cfg.Output.DB)
	assert.Equal(t, "postgres", cfg.Dialect)
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drel.yaml")
	os.WriteFile(path, []byte("packages:\n  - ./models\n"), 0644)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "./db/drel_gen.go", cfg.Output.DB)
	assert.Equal(t, "postgres", cfg.Dialect)
}

func TestLoadConfig_NoPackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drel.yaml")
	os.WriteFile(path, []byte("dialect: postgres"), 0644)
	_, err := LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no packages")
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/drel.yaml")
	assert.Error(t, err)
}
