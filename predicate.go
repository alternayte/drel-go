package drel

import (
	"errors"
	"fmt"

	"github.com/alternayte/drel/internal/ast"
)

// ErrRawArgMismatch is returned by RawErr when the number of ? placeholders in
// the raw SQL does not match the number of supplied arguments.
var ErrRawArgMismatch = errors.New("drel: raw predicate placeholder/argument count mismatch")

type Predicate struct {
	clause ast.WhereClause
}

func newComparison(column string, op ast.Operator, value any) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     op,
				Value:  value,
			},
		},
	}
}

func newInComparison(column string, values []any) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpIn,
				Values: values,
			},
		},
	}
}

func newNotInComparison(column string, values []any) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpNotIn,
				Values: values,
			},
		},
	}
}

func newBetweenComparison[T any](column string, low, high T) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpBetween,
				Values: []any{low, high},
			},
		},
	}
}

func newNullCheck(column string, isNull bool) Predicate {
	op := ast.OpIsNull
	if !isNull {
		op = ast.OpIsNotNull
	}
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     op,
			},
		},
	}
}

func And(preds ...Predicate) Predicate {
	children := make([]ast.WhereClause, len(preds))
	for i, p := range preds {
		children[i] = p.clause
	}
	return Predicate{
		clause: ast.WhereClause{
			LogicalOp: ast.LogicalAnd,
			Children:  children,
		},
	}
}

func Or(preds ...Predicate) Predicate {
	children := make([]ast.WhereClause, len(preds))
	for i, p := range preds {
		children[i] = p.clause
	}
	return Predicate{
		clause: ast.WhereClause{
			LogicalOp: ast.LogicalOr,
			Children:  children,
		},
	}
}

func Not(pred Predicate) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			LogicalOp: ast.LogicalNot,
			Children:  []ast.WhereClause{pred.clause},
		},
	}
}

// True returns a no-op predicate that always matches. It is the identity for
// And and contributes nothing to a WHERE clause, so Where(True()) emits no
// WHERE at all. Use it as the "else" branch of conditional filtering.
func True() Predicate {
	return Predicate{}
}

// WhereIf returns pred when cond is true, otherwise a no-op (always-true)
// predicate that contributes nothing to the WHERE clause. It makes conditional
// filtering first-class instead of forcing callers to construct zero Predicate{}
// values or hand-rolled if/else chains around the builder.
func WhereIf(cond bool, pred Predicate) Predicate {
	if cond {
		return pred
	}
	return True()
}

// countRawPlaceholders counts '?' placeholders in a raw SQL fragment, ignoring
// any '?' inside single-quoted strings, double-quoted identifiers, or
// dollar-quoted ($$...$$) regions. It uses the same scanner state machine as the
// dialect emitters so the two can never disagree about placeholder count.
func countRawPlaceholders(sql string) int {
	count := 0
	state := 0 // 0=normal, 1=single-quote, 2=double-quote, 3=dollar-quote
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch state {
		case 0: // normal
			switch {
			case ch == '\'':
				state = 1
			case ch == '"':
				state = 2
			case ch == '$' && i+1 < len(sql) && sql[i+1] == '$':
				state = 3
				i++
			case ch == '?':
				count++
			}
		case 1: // single-quoted string
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++ // escaped ''
				} else {
					state = 0
				}
			}
		case 2: // double-quoted identifier
			if ch == '"' {
				state = 0
			}
		case 3: // dollar-quoted string
			if ch == '$' && i+1 < len(sql) && sql[i+1] == '$' {
				i++
				state = 0
			}
		}
	}
	return count
}

// Raw creates a predicate from a raw SQL expression with bound arguments.
// Use ? as placeholder for each argument; they are rewritten to $N for Postgres.
// Placeholder counting is quote-aware: a '?' inside a single-quoted string,
// double-quoted identifier, or dollar-quoted region is not a placeholder.
// Panics if the number of placeholders does not match the number of arguments;
// use RawErr for a non-panicking variant.
func Raw(sql string, args ...any) Predicate {
	count := countRawPlaceholders(sql)
	if count != len(args) {
		panic(fmt.Sprintf("drel.Raw: %d placeholder(s) but %d argument(s)", count, len(args)))
	}
	return Predicate{
		clause: ast.WhereClause{
			Raw:     &sql,
			RawArgs: args,
		},
	}
}

// RawErr is like Raw but returns an error wrapping ErrRawArgMismatch on a
// placeholder/argument-count mismatch instead of panicking. Use it when the raw
// SQL or its argument list is built dynamically and a mismatch is recoverable.
func RawErr(sql string, args ...any) (Predicate, error) {
	count := countRawPlaceholders(sql)
	if count != len(args) {
		return Predicate{}, fmt.Errorf("%w: %d placeholder(s) but %d argument(s)", ErrRawArgMismatch, count, len(args))
	}
	return Predicate{
		clause: ast.WhereClause{
			Raw:     &sql,
			RawArgs: args,
		},
	}, nil
}

func (p Predicate) ToAST() ast.WhereClause {
	return p.clause
}
