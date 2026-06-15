package codegen

import (
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiColVO_ScanEmitFormat(t *testing.T) {
	dir := setupTestModule(t, map[string]string{
		"models/model.go": "package models\n\n" +
			"import \"github.com/alternayte/drel\"\n\n" +
			"type Money struct {\n\tamount   int\n\tcurrency string\n}\n\n" +
			"func NewMoney(amount int, currency string) Money { return Money{amount: amount, currency: currency} }\n" +
			"func (m Money) Amount() int      { return m.amount }\n" +
			"func (m Money) Currency() string { return m.currency }\n" +
			"func (m Money) DrelColumns() []string      { return []string{\"amount\", \"currency\"} }\n" +
			"func (m Money) DrelValues() ([]any, error) { return []any{m.amount, m.currency}, nil }\n" +
			"func (m *Money) DrelScanMulti(v []any) error {\n" +
			"\tif len(v) != 2 { return nil }\n" +
			"\tif a, ok := v[0].(int); ok { m.amount = a }\n" +
			"\tif c, ok := v[1].(string); ok { m.currency = c }\n" +
			"\treturn nil\n}\n\n" +
			"type Account struct {\n\tdrel.Model[int]\n\tname    string " + "`db:\"name\"`" + "\n\tbalance Money  " + "`db:\"balance_amount,balance_currency\"`" + "\n}\n",
	})

	models, err := ScanPackages([]string{filepath.Join(dir, "models")}, dir)
	require.NoError(t, err)
	require.Len(t, models, 1)

	m := models[0]
	balance := m.Fields[1]
	require.True(t, balance.IsMultiColVO)
	require.Equal(t, []string{"balance_amount", "balance_currency"}, balance.MultiColNames)

	out, err := EmitModelFileChecked(m)
	require.NoError(t, err)

	// gofmt must succeed (proves the generated source is well-formed Go).
	formatted, ferr := format.Source([]byte(out))
	require.NoError(t, ferr, "generated code did not gofmt:\n%s", out)

	// Parse to be doubly sure and check key constructs are present.
	_, perr := parser.ParseFile(token.NewFileSet(), "account_drel.go", formatted, parser.AllErrors)
	require.NoError(t, perr)

	s := string(formatted)
	assert.Contains(t, s, "func accountMultiVals(v Money) []any")
	assert.Contains(t, s, "p.balance.DrelScanMulti(balanceVals)")
	// gofmt aligns struct field declarations with spaces between name and type;
	// check that each sub-column field is declared as drel.Column[any].
	assert.Regexp(t, `BalanceAmount\s+drel\.Column\[any\]`, s)
	assert.Regexp(t, `BalanceCurrency\s+drel\.Column\[any\]`, s)
}
