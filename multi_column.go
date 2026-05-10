package drel

// MultiColumnMapper is implemented by value objects that map to multiple
// database columns (e.g., Money → amount + currency).
type MultiColumnMapper interface {
	DrelColumns() []string
	DrelValues() ([]any, error)
	DrelScanMulti(vals []any) error
}
