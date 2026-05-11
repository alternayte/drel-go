package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGenerateModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	goVersion := strings.TrimPrefix(runtime.Version(), "go")
	parts := strings.SplitN(goVersion, ".", 3)
	goDirective := parts[0] + "." + parts[1]

	moduleRoot := findModuleRoot(t)

	goMod := "module testmod\n\ngo " + goDirective + "\n\n" +
		"require github.com/alternayte/drel v0.0.0\n\n" +
		"replace github.com/alternayte/drel => " + moduleRoot + "\n"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644))

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}

	// Run go mod tidy so packages.Load can resolve dependencies.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidyOut, err := tidy.CombinedOutput()
	require.NoError(t, err, "go mod tidy in setup failed: %s", string(tidyOut))

	return dir
}

func TestGenerate_EndToEnd(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/product.go": `package models

import "github.com/alternayte/drel"

type Product struct {
	drel.Model[int]
	name    string ` + "`db:\"name\"`" + `
	price   int    ` + "`db:\"price\"`" + `
	inStock bool   ` + "`db:\"in_stock\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./models
output:
  db: ./db/drel_gen.go
`,
	})

	// Save and restore working directory.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	// Run the full codegen pipeline.
	err = Generate("drel.yaml")
	require.NoError(t, err)

	// Verify model file was generated.
	modelFile := filepath.Join(dir, "models", "product_drel.go")
	assert.FileExists(t, modelFile)

	modelContent, err := os.ReadFile(modelFile)
	require.NoError(t, err)
	modelStr := string(modelContent)

	assert.Contains(t, modelStr, "var Products = struct {")
	assert.Contains(t, modelStr, "var ProductMeta = drel.ModelMeta[Product]{")
	assert.Contains(t, modelStr, "Name drel.StringColumn")
	assert.Contains(t, modelStr, "Price drel.OrderedColumn[int]")
	assert.Contains(t, modelStr, "InStock drel.BoolColumn")

	// Verify DB file was generated.
	dbFile := filepath.Join(dir, "db", "drel_gen.go")
	assert.FileExists(t, dbFile)

	dbContent, err := os.ReadFile(dbFile)
	require.NoError(t, err)
	dbStr := string(dbContent)

	assert.Contains(t, dbStr, "type DB struct {")
	assert.Contains(t, dbStr, "Products *models.ProductRepository")

	// Run go mod tidy and go build to verify generated code compiles.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidyOut, err := tidy.CombinedOutput()
	require.NoError(t, err, "go mod tidy failed: %s", string(tidyOut))

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOut))
}

func TestGenerate_MultipleModels(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/user.go": `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name  string ` + "`db:\"name\"`" + `
	email string ` + "`db:\"email\"`" + `
}
`,
		"models/post.go": `package models

import "github.com/alternayte/drel"

type Post struct {
	drel.Model[int]
	title string ` + "`db:\"title\"`" + `
	body  string ` + "`db:\"body\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./models
output:
  db: ./db/drel_gen.go
`,
	})

	// Save and restore working directory.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	// Run the full codegen pipeline.
	err = Generate("drel.yaml")
	require.NoError(t, err)

	// Verify both model files were generated.
	userFile := filepath.Join(dir, "models", "user_drel.go")
	postFile := filepath.Join(dir, "models", "post_drel.go")
	assert.FileExists(t, userFile)
	assert.FileExists(t, postFile)

	// Verify DB file has both repositories.
	dbFile := filepath.Join(dir, "db", "drel_gen.go")
	assert.FileExists(t, dbFile)

	dbContent, err := os.ReadFile(dbFile)
	require.NoError(t, err)
	dbStr := string(dbContent)

	assert.Contains(t, dbStr, "type DB struct {")
	assert.Contains(t, dbStr, "Users *models.UserRepository")
	assert.Contains(t, dbStr, "Posts *models.PostRepository")

	// Run go mod tidy and go build to verify generated code compiles.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidyOut, err := tidy.CombinedOutput()
	require.NoError(t, err, "go mod tidy failed: %s", string(tidyOut))

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOut))
}

func TestGenerate_WithValueObjects(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/types.go": `package models

import (
	"database/sql/driver"
	"fmt"
)

type Email struct{ address string }

func NewEmail(addr string) Email { return Email{address: addr} }
func (e Email) String() string   { return e.address }
func (e Email) Value() (driver.Value, error) { return e.address, nil }
func (e *Email) Scan(src any) error {
	s, ok := src.(string)
	if !ok { return fmt.Errorf("Email.Scan: expected string, got %T", src) }
	e.address = s
	return nil
}

type Role string
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)
`,
		"models/user.go": `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name  string ` + "`db:\"name\"`" + `
	email Email  ` + "`db:\"email\"`" + `
	role  Role   ` + "`db:\"role\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./models
output:
  db: ./db/drel_gen.go
`,
	})

	// Save and restore working directory.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	// Run the full codegen pipeline.
	err = Generate("drel.yaml")
	require.NoError(t, err)

	// Verify model file was generated.
	modelFile := filepath.Join(dir, "models", "user_drel.go")
	assert.FileExists(t, modelFile)

	modelContent, err := os.ReadFile(modelFile)
	require.NoError(t, err)
	modelStr := string(modelContent)

	// Verify VO type uses unqualified name (Email, not testmod/models.Email).
	assert.Contains(t, modelStr, "drel.Column[Email]")
	assert.Contains(t, modelStr, `drel.NewCol[Email]("email")`)

	// Verify string-based enum type uses unqualified name.
	assert.Contains(t, modelStr, "drel.Column[Role]")
	assert.Contains(t, modelStr, `drel.NewCol[Role]("role")`)

	// Verify snapshot uses local types.
	assert.Contains(t, modelStr, "email Email")
	assert.Contains(t, modelStr, "role Role")

	// Run go mod tidy and go build to verify generated code compiles.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidyOut, err := tidy.CombinedOutput()
	require.NoError(t, err, "go mod tidy failed: %s", string(tidyOut))

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOut))
}
