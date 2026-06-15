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
		"models/model.go": "package models\n\nimport \"github.com/alternayte/drel\"\n\ntype Money struct {\n\tamount   int\n\tcurrency string\n}\n\nfunc (m Money) DrelColumns() []string        { return []string{\"amount\", \"currency\"} }\nfunc (m Money) DrelValues() ([]any, error)   { return []any{m.amount, m.currency}, nil }\nfunc (m *Money) DrelScanMulti(v []any) error { return nil }\n\ntype Product struct {\n\tdrel.Model[int]\n\tname    string " + "`db:\"name\"`" + "\n\tbalance Money  " + "`db:\"balance\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	m := models[0]
	require.Len(t, m.Fields, 2)

	balanceField := m.Fields[1]
	assert.Equal(t, "balance", balanceField.Name)
	assert.True(t, balanceField.IsMultiColVO)
	assert.Equal(t, "balance", balanceField.MultiColPrefix)
	assert.Equal(t, "Money", balanceField.LocalGoType)
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
