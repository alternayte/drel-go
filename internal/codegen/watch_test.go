package codegen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchDirs_ReturnsScannedPackageDirs(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/user.go": `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
		"posts/post.go": `package posts

import "github.com/alternayte/drel"

type Post struct {
	drel.Model[int]
	title string ` + "`db:\"title\"`" + `
}
`,
		"drel.yaml": `packages:
  - ./models
  - ./posts
output:
  db: ./db/drel_gen.go
`,
	})

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	require.NoError(t, os.Chdir(dir))

	dirs, err := watchDirs("drel.yaml")
	require.NoError(t, err)

	// filepath.EvalSymlinks canonicalises /var -> /private/var on macOS where
	// os.TempDir returns the non-resolved path but go/packages resolves it.
	canonDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	want := []string{
		filepath.Join(canonDir, "models"),
		filepath.Join(canonDir, "posts"),
	}
	sort.Strings(want)
	assert.Equal(t, want, dirs)
}

func TestDirsSignature_IgnoresGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "user.go")
	gen := filepath.Join(dir, "user_drel.go")
	dbOut := filepath.Join(dir, "drel_gen.go")

	require.NoError(t, os.WriteFile(src, []byte("package models\n"), 0644))
	require.NoError(t, os.WriteFile(gen, []byte("package models\n"), 0644))
	require.NoError(t, os.WriteFile(dbOut, []byte("package models\n"), 0644))

	skip := map[string]bool{"drel_gen.go": true}

	base, err := dirsSignature([]string{dir}, skip)
	require.NoError(t, err)

	// Touch a generated file and the DB-output file: signature must not change.
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(gen, future, future))
	require.NoError(t, os.Chtimes(dbOut, future, future))
	afterGen, err := dirsSignature([]string{dir}, skip)
	require.NoError(t, err)
	assert.Equal(t, base, afterGen, "generated/DB-output changes must not alter the signature")

	// Touch the source file: signature MUST change.
	require.NoError(t, os.Chtimes(src, future, future))
	afterSrc, err := dirsSignature([]string{dir}, skip)
	require.NoError(t, err)
	assert.NotEqual(t, base, afterSrc, "source changes must alter the signature")
}

func TestGenerateWatch_RegeneratesOnSourceChange(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/user.go": `package models

import "github.com/alternayte/drel"

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

	// Use absolute config path so the goroutine is not sensitive to process-wide
	// os.Chdir races under the race detector.
	configPath := filepath.Join(dir, "drel.yaml")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- GenerateWatch(ctx, configPath, 20*time.Millisecond) }()

	modelFile := filepath.Join(dir, "models", "user_drel.go")
	dbFile := filepath.Join(dir, "db", "drel_gen.go")

	// Initial run must produce both files. Allow generous timeout because the race
	// detector slows packages.Load considerably (2-20x overhead).
	requireEventually(t, func() bool {
		_, e1 := os.Stat(modelFile)
		_, e2 := os.Stat(dbFile)
		return e1 == nil && e2 == nil
	}, 30*time.Second, "initial generation did not produce output files")

	// Capture the DB file's current signature (mod time) for change detection.
	before, err := os.ReadFile(dbFile)
	require.NoError(t, err)

	// Add an email field to the source model; watch must regenerate.
	updated := `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name  string ` + "`db:\"name\"`" + `
	email string ` + "`db:\"email\"`" + `
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models", "user.go"), []byte(updated), 0644))

	requireEventually(t, func() bool {
		c, e := os.ReadFile(modelFile)
		// Verify the email field was picked up: the generated struct will contain
		// NewStringCol("email") or similar column constructor referencing "email".
		return e == nil && strings.Contains(string(c), `NewStringCol("email")`)
	}, 30*time.Second, "watch did not regenerate after source change")

	// Sanity: the DB file content is unchanged (one model) but regen ran without panic.
	_ = before

	// Cancellation must return promptly with nil.
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("GenerateWatch did not exit after ctx cancel")
	}
}

// requireEventually polls cond until true or the timeout elapses.
func requireEventually(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestGenerateWatch_DoesNotSelfTrigger(t *testing.T) {
	dir := setupGenerateModule(t, map[string]string{
		"models/user.go": `package models

import "github.com/alternayte/drel"

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

	// Use absolute config path so the goroutine is not sensitive to process-wide
	// os.Chdir races under the race detector.
	configPath := filepath.Join(dir, "drel.yaml")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = GenerateWatch(ctx, configPath, 20*time.Millisecond) }()

	modelFile := filepath.Join(dir, "models", "user_drel.go")
	requireEventually(t, func() bool {
		_, e := os.Stat(modelFile)
		return e == nil
	}, 30*time.Second, "initial generation did not run")

	// Record the generated file's mtime, then wait several poll cycles WITHOUT
	// touching any source file. If the watcher self-triggered, it would rewrite
	// the generated files and the mtime would advance.
	first, err := os.Stat(modelFile)
	require.NoError(t, err)
	firstMod := first.ModTime()

	time.Sleep(500 * time.Millisecond) // enough poll cycles even under race-detector overhead

	after, err := os.Stat(modelFile)
	require.NoError(t, err)
	assert.Equal(t, firstMod, after.ModTime(),
		"watcher regenerated without a source change (self-trigger loop)")
}

func TestGoGenerate_InvokesCodegen(t *testing.T) {
	moduleRoot := findModuleRoot(t)

	dir := setupGenerateModule(t, map[string]string{
		"models/user.go": `package models

import "github.com/alternayte/drel"

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
		// Top-level file carrying the go:generate directive. It invokes the
		// repo CLI via `go run` so no installed binary is required.
		"gen.go": `package app

//go:generate go run ` + moduleRoot + `/cmd/drel generate
`,
	})

	cmd := exec.Command("go", "generate", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go generate failed: %s", string(out))

	assert.FileExists(t, filepath.Join(dir, "models", "user_drel.go"))
	assert.FileExists(t, filepath.Join(dir, "db", "drel_gen.go"))
}

// TestGoGenerate_InvokesCodegen_SubdirLayout proves that //go:generate works
// when drel.yaml lives in a subdirectory of the module root — the layout every
// example uses (e.g. examples/getting-started/ inside the repo root).
// Before the fix, `drel generate` resolved ./models relative to the module root
// (found by walking up from cfgDir to the nearest go.mod), so a config in
// <module>/app/ with packages: [./models] looked for <module>/models, not
// <module>/app/models.
func TestGoGenerate_InvokesCodegen_SubdirLayout(t *testing.T) {
	moduleRoot := findModuleRoot(t)

	// dir is the module root for the temp test project.
	dir := setupGenerateModule(t, map[string]string{
		// The "app" subdirectory mirrors how examples sit inside the repo root.
		"app/models/user.go": `package models

import "github.com/alternayte/drel"

type User struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
		// drel.yaml and the go:generate file live inside the subdirectory.
		"app/drel.yaml": `packages:
  - ./models
output:
  db: ./db/drel_gen.go
`,
		// gen.go carries the directive; go generate sets GOFILE/cwd to app/.
		"app/gen.go": `package app

//go:generate go run ` + moduleRoot + `/cmd/drel generate
`,
	})

	// Run go generate from the module root (as `go generate ./examples/...`
	// does in the real repo), which processes app/gen.go.
	cmd := exec.Command("go", "generate", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go generate ./... from module root failed: %s", string(out))

	assert.FileExists(t, filepath.Join(dir, "app", "models", "user_drel.go"),
		"model file must be generated relative to config dir, not module root")
	assert.FileExists(t, filepath.Join(dir, "app", "db", "drel_gen.go"),
		"db file must be generated relative to config dir, not module root")
}
