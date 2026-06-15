package drel

import "strings"

func rewritePlaceholders(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	inQuote := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if ch == '\'' {
			inQuote = !inQuote
			b.WriteByte(ch)
			continue
		}
		if inQuote {
			b.WriteByte(ch)
			continue
		}
		if ch == '$' && i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9' {
			b.WriteByte('?')
			i++
			for i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9' {
				i++
			}
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// needsPlaceholderRewrite returns true if the engine's dialect binds parameters
// with "?" (i.e., SQLite/libSQL) and raw SQL written with $1, $2, ... needs
// rewriting to "?". This is keyed on an explicit dialect capability rather than
// the RETURNING flag so the two never drift apart.
func needsPlaceholderRewrite(e *Engine) bool {
	return e.dialect().UsesQuestionPlaceholders()
}
