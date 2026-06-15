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
		{"check with in-list", `db:"role,check=role IN ('admin','user')"`, "role", dbTagOpts{check: "role IN ('admin','user')"}},
		{"check with func commas", `db:"x,check=substr(x,1,2) = 'ab'"`, "x", dbTagOpts{check: "substr(x,1,2) = 'ab'"}},
		{"unique then check with comma", `db:"role,unique,check=role IN ('a','b')"`, "role", dbTagOpts{unique: true, check: "role IN ('a','b')"}},
		{"default string", `db:"role,default=user"`, "role", dbTagOpts{def: "user"}},
		{"default with comma value", `db:"flags,default=ARRAY['a','b']"`, "flags", dbTagOpts{def: "ARRAY['a','b']"}},
		{"type override", `db:"meta,type=jsonb"`, "meta", dbTagOpts{typ: "jsonb"}},
		{"unique then default", `db:"role,unique,default=user"`, "role", dbTagOpts{unique: true, def: "user"}},
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

func TestScanner_DefaultAndTypeTags(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Account struct {
	drel.Model[int]
	role string ` + "`db:\"role,default=user\"`" + `
	meta string ` + "`db:\"meta,type=jsonb\"`" + `
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
	assert.Equal(t, "user", byCol["role"].Default)
	assert.Equal(t, "jsonb", byCol["meta"].TypeOverride)
}

func TestScanner_RejectsUnsignedPK(t *testing.T) {
	for _, pk := range []string{"uint", "uint8", "uint16", "uint32", "uint64"} {
		t.Run(pk, func(t *testing.T) {
			dir := setupTestModule(t, map[string]string{
				"models/model.go": `package models

import "github.com/alternayte/drel"

type Widget struct {
	drel.Model[` + pk + `]
	name string ` + "`db:\"name\"`" + `
}
`,
			})

			_, err := ScanPackages([]string{"./models"}, dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsigned integer primary keys")
			assert.Contains(t, err.Error(), "Widget")
			assert.Contains(t, err.Error(), pk)
		})
	}
}

func TestScanner_AcceptsSignedAndUUIDPK(t *testing.T) {
	// Signed integer PKs (the common case) must pass the guard.
	// UUID PKs are covered by the emitter tests; we don't import uuid here
	// because the test module's go.mod only resolves via drel's replace directive.
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Signed struct {
	drel.Model[int64]
	name string ` + "`db:\"name\"`" + `
}

type Also struct {
	drel.Model[int]
	name string ` + "`db:\"name\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 2)
}

func TestScanner_StringEnum_PreservesDeclarationOrder(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Status string

const (
	StatusNew      Status = "new"
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusZebra    Status = "zebra"
)

type Order struct {
	drel.Model[int]
	status Status ` + "`db:\"status\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	var f *FieldInfo
	for i := range models[0].Fields {
		if models[0].Fields[i].ColumnName == "status" {
			f = &models[0].Fields[i]
		}
	}
	require.NotNil(t, f)
	assert.True(t, f.IsEnum)
	assert.False(t, f.EnumIsInt)
	assert.Equal(t, "string", f.EnumBaseType)
	// Declaration order preserved, NOT alphabetized ("approved" would sort first).
	assert.Equal(t, []string{"new", "pending", "approved", "zebra"}, f.EnumValues)
}

func TestScanner_IntEnum_KindAndOrder(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Priority int

const (
	PriorityLow    Priority = 0
	PriorityMedium Priority = 1
	PriorityHigh   Priority = 2
)

type Ticket struct {
	drel.Model[int]
	priority Priority ` + "`db:\"priority\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	var f *FieldInfo
	for i := range models[0].Fields {
		if models[0].Fields[i].ColumnName == "priority" {
			f = &models[0].Fields[i]
		}
	}
	require.NotNil(t, f)
	assert.True(t, f.IsEnum)
	assert.True(t, f.EnumIsInt)
	assert.Equal(t, "int", f.EnumBaseType)
	assert.Equal(t, []string{"0", "1", "2"}, f.EnumValues)
}

func TestParseDBTag_Default(t *testing.T) {
	col, opts := parseDBTag(`db:"role,default=user"`)
	assert.Equal(t, "role", col)
	assert.Equal(t, "user", opts.def)
}

func TestScanner_EnumDefault_RecordedOnField(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import "github.com/alternayte/drel"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type Account struct {
	drel.Model[int]
	role Role ` + "`db:\"role,default=user\"`" + `
}
`,
	})
	models, err := ScanPackages([]string{"./models"}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	var f *FieldInfo
	for i := range models[0].Fields {
		if models[0].Fields[i].ColumnName == "role" {
			f = &models[0].Fields[i]
		}
	}
	require.NotNil(t, f)
	assert.Equal(t, "user", f.Default)
}
