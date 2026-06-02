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

func TestScanner_RelTagParsing(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/user.go": "package models\n\nimport \"github.com/alternayte/drel\"\n\ntype Post struct {\n\tdrel.Model[int]\n\ttitle string " + "`db:\"title\"`" + "\n}\n\ntype User struct {\n\tdrel.Model[int]\n\tname  string " + "`db:\"name\"`" + "\n\tposts []Post " + "`rel:\"has_many,fk=user_id\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)

	var user *ModelInfo
	for i := range models {
		if models[i].Name == "User" {
			user = &models[i]
			break
		}
	}
	require.NotNil(t, user)
	require.Len(t, user.Fields, 2)

	postsField := user.Fields[1]
	assert.Equal(t, "posts", postsField.Name)
	assert.Equal(t, "", postsField.ColumnName)
	require.NotNil(t, postsField.Relation)
	assert.Equal(t, "has_many", postsField.Relation.Type)
	assert.Equal(t, "user_id", postsField.Relation.FK)
}

func TestParseRelTagStructured(t *testing.T) {
	tests := []struct {
		tag  string
		want *RelationFieldInfo
	}{
		{"has_many,fk=user_id", &RelationFieldInfo{Type: "has_many", FK: "user_id"}},
		{"has_one,fk=user_id", &RelationFieldInfo{Type: "has_one", FK: "user_id"}},
		{"belongs_to,fk=user_id", &RelationFieldInfo{Type: "belongs_to", FK: "user_id"}},
		{"many_to_many,join=user_tags", &RelationFieldInfo{Type: "many_to_many", JoinTable: "user_tags"}},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := parseRelTagStructured(tt.tag)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRelTag_ManyToManyConventionDefaults(t *testing.T) {
	ri := parseRelTagStructured("many_to_many")
	assert.Equal(t, "many_to_many", ri.Type)
	assert.Empty(t, ri.FK)
	assert.Empty(t, ri.JoinTable)
	assert.Empty(t, ri.RefColumn)
}

func TestParseRelTag_ManyToManyWithRef(t *testing.T) {
	ri := parseRelTagStructured("many_to_many,join=taggings,fk=writer_id,ref=label_id")
	assert.Equal(t, "many_to_many", ri.Type)
	assert.Equal(t, "taggings", ri.JoinTable)
	assert.Equal(t, "writer_id", ri.FK)
	assert.Equal(t, "label_id", ri.RefColumn)
}

func TestParseDBTag_Options(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		col  string
		opts dbTagOpts
	}{
		{"unique", `db:"email,unique"`, "email", dbTagOpts{unique: true}},
		{"index", `db:"age,index"`, "age", dbTagOpts{indexed: true}},
		{"named index", `db:"x,index=ix"`, "x", dbTagOpts{indexed: true, indexName: "ix"}},
		{"check", `db:"y,check=y > 0"`, "y", dbTagOpts{check: "y > 0"}},
		{"plain", `db:"name"`, "name", dbTagOpts{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, opts := parseDBTag(tt.tag)
			assert.Equal(t, tt.col, col)
			assert.Equal(t, tt.opts, opts)
		})
	}
}

func TestScanner_IndexAndCheckTags(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Account struct {
	drel.Model[int]
	email string ` + "`db:\"email,unique\"`" + `
	age   int    ` + "`db:\"age,index\"`" + `
	first string ` + "`db:\"first,index=ix_name\"`" + `
	score int    ` + "`db:\"score,check=score > 0\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	byCol := map[string]FieldInfo{}
	for _, f := range models[0].Fields {
		byCol[f.ColumnName] = f
	}

	assert.True(t, byCol["email"].Unique)
	assert.True(t, byCol["age"].Indexed)
	assert.Empty(t, byCol["age"].IndexName)
	assert.True(t, byCol["first"].Indexed)
	assert.Equal(t, "ix_name", byCol["first"].IndexName)
	assert.Equal(t, "score > 0", byCol["score"].CheckExpr)
}
