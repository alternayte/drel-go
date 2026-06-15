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
	modelNorm := strings.Join(strings.Fields(modelStr), " ") // gofmt-alignment-insensitive

	assert.Contains(t, modelStr, "var Products = struct {")
	assert.Contains(t, modelStr, "var ProductMeta = drel.ModelMeta[Product]{")
	assert.Contains(t, modelNorm, "Name drel.StringColumn")
	assert.Contains(t, modelNorm, "Price drel.OrderedColumn[int]")
	assert.Contains(t, modelNorm, "InStock drel.BoolColumn")

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

func TestResolveModuleRoot_IsExportedForMigrateNew(t *testing.T) {
	// migrate new must scan from the module root, not the config dir, identical
	// to Generate. This guards that the shared helper exists and is exported so
	// cmd/drel/migrate.go can call it.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module m\n\ngo 1.22\n"), 0644))
	sub := filepath.Join(root, "deploy")
	require.NoError(t, os.MkdirAll(sub, 0755))
	assert.Equal(t, root, ResolveModuleRoot(sub))
}

func TestResolveModuleRoot_ConfigInSubdir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module testmod\n\ngo 1.22\n"), 0644))
	sub := filepath.Join(root, "config", "nested")
	require.NoError(t, os.MkdirAll(sub, 0755))

	got := ResolveModuleRoot(sub)
	assert.Equal(t, root, got)
}

func TestResolveModuleRoot_ConfigAtRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module testmod\n\ngo 1.22\n"), 0644))

	got := ResolveModuleRoot(root)
	assert.Equal(t, root, got)
}

func TestResolveModuleRoot_NoGoMod_FallsBackToStart(t *testing.T) {
	// A directory with no go.mod anywhere above it falls back to startDir,
	// preserving today's behaviour for configs that *are* at the module root.
	start := filepath.Join(t.TempDir(), "x", "y")
	require.NoError(t, os.MkdirAll(start, 0755))

	got := ResolveModuleRoot(start)
	assert.Equal(t, start, got)
}

func TestGenerate_ConfigInSubdirOfModule(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		// Models live under the config subdirectory — packages in drel.yaml are
		// resolved relative to the config file's directory, not the module root.
		"config/models/product.go": `package models

import "github.com/alternayte/drel"

type Product struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
		"config/drel.yaml": `packages:
  - ./models
output:
  db: ./db/drel_gen.go
`,
	})

	// Config lives in <module>/config; ./models resolves to <module>/config/models.
	err := Generate(filepath.Join(dir, "config", "drel.yaml"))
	require.NoError(t, err)

	// Model file is written next to the scanned package (config-dir-rooted).
	assert.FileExists(t, filepath.Join(dir, "config", "models", "product_drel.go"))
	// DB output stays anchored to the config's directory.
	assert.FileExists(t, filepath.Join(dir, "config", "db", "drel_gen.go"))
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
	modelNorm := strings.Join(strings.Fields(modelStr), " ") // gofmt-alignment-insensitive

	// Verify VO type uses unqualified name (Email, not testmod/models.Email).
	assert.Contains(t, modelStr, "drel.Column[Email]")
	assert.Contains(t, modelStr, `drel.NewCol[Email]("email")`)

	// Verify string-based enum type uses unqualified name.
	assert.Contains(t, modelStr, "drel.Column[Role]")
	assert.Contains(t, modelStr, `drel.NewCol[Role]("role")`)

	// Verify snapshot uses local types.
	assert.Contains(t, modelNorm, "email Email")
	assert.Contains(t, modelNorm, "role Role")

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

func TestGenerate_DuplicateModelName(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"auth/user.go": `package auth

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
		"billing/user.go": `package billing

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	plan string ` + "`db:\"plan\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./auth
  - ./billing
output:
  db: ./db/drel_gen.go
`,
	})

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	err = Generate("drel.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Users")
	// No DB file should exist after a validation failure.
	assert.NoFileExists(t, filepath.Join(dir, "db", "drel_gen.go"))
}

func TestGenerate_UnresolvedTarget(t *testing.T) {
	// Post references Author (from the authors package) via rel tag, but the
	// authors package is NOT listed in drel.yaml. The scanner loads the posts
	// package cleanly (imports resolve), but ValidateModels must catch "Author" as
	// an unresolved target and return an error naming the field and drel.yaml.
	dir := setupGenerateModule(t, map[string]string{
		"authors/author.go": `package authors

import "github.com/alternayte/drel"

type Author struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
		"posts/post.go": `package posts

import (
	"github.com/alternayte/drel"
	"testmod/authors"
)

type Post struct {
	drel.Model[int]
	title  string          ` + "`db:\"title\"`" + `
	Author *authors.Author ` + "`rel:\"belongs_to,fk=author_id\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./posts
output:
  db: ./db/drel_gen.go
`,
	})

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	err = Generate("drel.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Author")
	assert.Contains(t, err.Error(), "drel.yaml")
}

func TestGenerate_ColumnLessModel(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/models.go": `package models

import "github.com/alternayte/drel"

type Org struct {
	drel.Model[int]
	Members []*User ` + "`rel:\"has_many,fk=org_id\"`" + `
}

type User struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./models
output:
  db: ./db/drel_gen.go
`,
	})

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	err = Generate("drel.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no db-mapped columns")
}
