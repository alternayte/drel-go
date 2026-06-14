package drel

import "errors"

// ErrUnknownProjectionColumn is returned by Select/GroupBy when a projected SQL
// column has no DTO field whose `db` tag matches it. Binding such a column would
// either error confusingly or, when types align, silently corrupt data, so drel
// fails loudly instead. Match it with errors.Is.
var ErrUnknownProjectionColumn = errors.New("drel: projected column has no matching DTO field (db tag)")
