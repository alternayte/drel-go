package codegen

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	goVer := strings.TrimPrefix(runtime.Version(), "go")
	goMod := "module testmod\n\ngo " + goVer + "\n\nrequire github.com/alternayte/drel v0.0.0\n\nreplace github.com/alternayte/drel => " + findModuleRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644))
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}
	return dir
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root")
		}
		dir = parent
	}
}

func TestScanner_SimpleModel(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Product struct {
	drel.Model[int]
	name    string ` + "`db:\"name\"`" + `
	price   int    ` + "`db:\"price\"`" + `
	inStock bool   ` + "`db:\"in_stock\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	m := models[0]
	assert.Equal(t, "Product", m.Name)
	assert.Equal(t, "int", m.PKType)
	assert.Equal(t, "products", m.TableName)
	assert.False(t, m.HasSoftDelete)
	assert.False(t, m.HasVersioned)

	require.Len(t, m.Fields, 3)
	assert.Equal(t, "name", m.Fields[0].Name)
	assert.Equal(t, "name", m.Fields[0].ColumnName)
	assert.Equal(t, "string", m.Fields[0].GoType)
	assert.Equal(t, "price", m.Fields[1].ColumnName)
	assert.Equal(t, "in_stock", m.Fields[2].ColumnName)
}

func TestScanner_SoftDeleteEmbed(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Article struct {
	drel.Model[int]
	drel.SoftDelete
	title string ` + "`db:\"title\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.True(t, models[0].HasSoftDelete)
	assert.Equal(t, "articles", models[0].TableName)
}

func TestScanner_VersionedEmbed(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Item struct {
	drel.Model[int]
	drel.Versioned
	label string ` + "`db:\"label\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.True(t, models[0].HasVersioned)
}

func TestScanner_NoModelEmbedSkipped(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

type NotAModel struct {
	Name string
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestScanner_FieldWithoutDBTagSkipped(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name   string ` + "`db:\"name\"`" + `
	secret string
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Len(t, models[0].Fields, 1)
	assert.Equal(t, "name", models[0].Fields[0].ColumnName)
}

func TestScanner_MultipleModels(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}

type Post struct {
	drel.Model[int]
	title string ` + "`db:\"title\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	assert.Len(t, models, 2)
}
