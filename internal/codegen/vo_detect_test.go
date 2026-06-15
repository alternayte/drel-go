package codegen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanner_SingleColVO(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": "package models\n\nimport (\n\t\"database/sql/driver\"\n\t\"fmt\"\n\t\"github.com/alternayte/drel\"\n)\n\ntype Email struct{ address string }\n\nfunc (e Email) Value() (driver.Value, error) { return e.address, nil }\nfunc (e *Email) Scan(src any) error {\n\ts, ok := src.(string)\n\tif !ok { return fmt.Errorf(\"expected string\") }\n\te.address = s\n\treturn nil\n}\n\ntype User struct {\n\tdrel.Model[int]\n\tname  string " + "`db:\"name\"`" + "\n\temail Email  " + "`db:\"email\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	m := models[0]
	require.Len(t, m.Fields, 2)

	nameField := m.Fields[0]
	assert.Equal(t, "name", nameField.Name)
	assert.False(t, nameField.IsVO)

	emailField := m.Fields[1]
	assert.Equal(t, "email", emailField.Name)
	assert.True(t, emailField.IsVO)
	assert.False(t, emailField.IsMultiColVO)
	assert.Equal(t, "Email", emailField.LocalGoType)
}

func TestScanner_MultiColVO(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": "package models\n\nimport \"github.com/alternayte/drel\"\n\ntype Money struct {\n\tamount   int\n\tcurrency string\n}\n\nfunc (m Money) DrelColumns() []string        { return []string{\"amount\", \"currency\"} }\nfunc (m Money) DrelValues() ([]any, error)   { return []any{m.amount, m.currency}, nil }\nfunc (m *Money) DrelScanMulti(v []any) error { return nil }\n\ntype Product struct {\n\tdrel.Model[int]\n\tname    string " + "`db:\"name\"`" + "\n\tbalance Money  " + "`db:\"balance_amount,balance_currency\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	m := models[0]
	require.Len(t, m.Fields, 2)

	balanceField := m.Fields[1]
	assert.Equal(t, "balance", balanceField.Name)
	assert.True(t, balanceField.IsMultiColVO)
	assert.Equal(t, "Money", balanceField.LocalGoType)
	// The first sub-column name is recorded as ColumnName so columnFields()
	// keeps including the field; the full expansion lives in MultiColNames.
	assert.Equal(t, "balance_amount", balanceField.ColumnName)
	assert.Equal(t, []string{"balance_amount", "balance_currency"}, balanceField.MultiColNames)
	assert.Equal(t, []string{"text", "text"}, balanceField.MultiColTypes)
	// Multi-col VO fields must NOT run option parsing: a comma list is column
	// names, not options, so Unique/Indexed/CheckExpr stay zero.
	assert.False(t, balanceField.Unique)
	assert.False(t, balanceField.Indexed)
	assert.Empty(t, balanceField.CheckExpr)
}

func TestScanner_MultiColTypesDefaultText(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": "package models\n\nimport \"github.com/alternayte/drel\"\n\ntype Money struct {\n\tamount   int\n\tcurrency string\n}\n\nfunc (m Money) DrelColumns() []string        { return []string{\"amount\", \"currency\"} }\nfunc (m Money) DrelValues() ([]any, error)   { return []any{m.amount, m.currency}, nil }\nfunc (m *Money) DrelScanMulti(v []any) error { return nil }\n\ntype Product struct {\n\tdrel.Model[int]\n\tname    string " + "`db:\"name\"`" + "\n\tbalance Money  " + "`db:\"balance_amount,balance_currency\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	balanceField := models[0].Fields[1]
	require.True(t, balanceField.IsMultiColVO)
	assert.Equal(t, []string{"balance_amount", "balance_currency"}, balanceField.MultiColNames)
	assert.Equal(t, []string{"text", "text"}, balanceField.MultiColTypes)
}

func TestScanner_SingleColVO_HasIsZero(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import (
	"database/sql/driver"
	"fmt"

	"github.com/alternayte/drel"
)

type Email struct{ address string }

func (e Email) Value() (driver.Value, error) {
	if e.address == "" {
		return nil, nil
	}
	return e.address, nil
}
func (e *Email) Scan(src any) error {
	if src == nil {
		e.address = ""
		return nil
	}
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string")
	}
	e.address = s
	return nil
}
func (e Email) IsZero() bool { return e.address == "" }

type Account struct {
	drel.Model[int]
	email Email ` + "`db:\"email\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Len(t, models[0].Fields, 1)
	assert.True(t, models[0].Fields[0].IsVO)
	assert.True(t, models[0].Fields[0].HasIsZero)
}

func TestScanner_SingleColVO_Comparable(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import (
	"database/sql/driver"
	"fmt"

	"github.com/alternayte/drel"
)

// Email is comparable (string-backed struct).
type Email struct{ address string }

func (e Email) Value() (driver.Value, error) { return e.address, nil }
func (e *Email) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string")
	}
	e.address = s
	return nil
}

// Tags holds a slice — NOT comparable with !=.
type Tags struct{ items []string }

func (tg Tags) Value() (driver.Value, error) { return "", nil }
func (tg *Tags) Scan(src any) error          { return nil }

type Account struct {
	drel.Model[int]
	email Email ` + "`db:\"email\"`" + `
	tags  Tags  ` + "`db:\"tags\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	fields := models[0].Fields
	require.Len(t, fields, 2)

	assert.True(t, fields[0].IsVO)
	assert.True(t, fields[0].IsComparable, "string-backed VO is comparable")

	assert.True(t, fields[1].IsVO)
	assert.False(t, fields[1].IsComparable, "slice-backed VO is not comparable")
}

func TestScanner_SingleColVO_HasEqual(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import (
	"database/sql/driver"
	"fmt"

	"github.com/alternayte/drel"
)

// Email has no Equal method.
type Email struct{ address string }

func (e Email) Value() (driver.Value, error) { return e.address, nil }
func (e *Email) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string")
	}
	e.address = s
	return nil
}

// Tags is a slice-backed VO with an Equal method.
type Tags struct{ items []string }

func (tg Tags) Value() (driver.Value, error) { return "", nil }
func (tg *Tags) Scan(src any) error          { return nil }
func (tg Tags) Equal(other Tags) bool {
	if len(tg.items) != len(other.items) {
		return false
	}
	for i := range tg.items {
		if tg.items[i] != other.items[i] {
			return false
		}
	}
	return true
}

type Account struct {
	drel.Model[int]
	email Email ` + "`db:\"email\"`" + `
	tags  Tags  ` + "`db:\"tags\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	fields := models[0].Fields
	require.Len(t, fields, 2)

	assert.True(t, fields[0].IsVO)
	assert.False(t, fields[0].HasEqual, "Email has no Equal method")

	assert.True(t, fields[1].IsVO)
	assert.True(t, fields[1].HasEqual, "Tags has an Equal method")
}

func TestScanner_SingleColVO_BaseType(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": `package models

import (
	"database/sql/driver"
	"fmt"

	"github.com/alternayte/drel"
)

type Email struct{ address string }

func (e Email) Value() (driver.Value, error) { return e.address, nil }
func (e *Email) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string")
	}
	e.address = s
	return nil
}

type Cents struct{ n int64 }

func (c Cents) Value() (driver.Value, error) { return c.n, nil }
func (c *Cents) Scan(src any) error {
	v, ok := src.(int64)
	if !ok {
		return fmt.Errorf("expected int64")
	}
	c.n = v
	return nil
}

type Account struct {
	drel.Model[int]
	email   Email ` + "`db:\"email\"`" + `
	balance Cents ` + "`db:\"balance\"`" + `
}
`,
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	fields := models[0].Fields
	require.Len(t, fields, 2)

	emailField := fields[0]
	assert.True(t, emailField.IsVO)
	assert.Equal(t, "string", emailField.VOBaseType)

	balanceField := fields[1]
	assert.True(t, balanceField.IsVO)
	assert.Equal(t, "int64", balanceField.VOBaseType)
}

func TestScanner_PrimitiveFieldNotVO(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": "package models\n\nimport \"github.com/alternayte/drel\"\n\ntype Item struct {\n\tdrel.Model[int]\n\tname  string " + "`db:\"name\"`" + "\n\tcount int    " + "`db:\"count\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	for _, f := range models[0].Fields {
		assert.False(t, f.IsVO, "field %s should not be VO", f.Name)
		assert.False(t, f.IsMultiColVO, "field %s should not be multi-col VO", f.Name)
	}
}
