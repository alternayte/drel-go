package drel

import (
	"testing"

	"github.com/alternayte/drel/internal/dialect/postgres"
	dialectsqlite "github.com/alternayte/drel/internal/dialect/sqlite"
)

func TestCountRawPlaceholders_QuoteAware(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want int
	}{
		{"single placeholder", "n = ?", 1},
		{"question mark in single-quoted literal", "note = 'huh?' AND n = ?", 1},
		{"two question marks both in literal", "a = 'x?y?' AND b = 1", 0},
		{"double-quoted identifier with question mark", `"weird?col" = ?`, 1},
		{"escaped single quote then placeholder", "a = 'it''s ok?' AND b = ?", 1},
		{"dollar-quoted region with question mark", "x = $$why?$$ AND y = ?", 1},
		{"no placeholders", "deleted_at IS NULL", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countRawPlaceholders(tt.sql)
			if got != tt.want {
				t.Errorf("countRawPlaceholders(%q) = %d, want %d", tt.sql, got, tt.want)
			}
		})
	}
}

func TestRaw_DoesNotPanicOnQuotedQuestionMark(t *testing.T) {
	// Must not panic: the only out-of-quote placeholder is the trailing ?.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Raw panicked unexpectedly: %v", r)
		}
	}()
	p := Raw("note = 'huh?' AND n = ?", 1)
	clause := p.ToAST()
	if clause.Raw == nil {
		t.Fatal("expected non-nil Raw clause")
	}
	if len(clause.RawArgs) != 1 {
		t.Fatalf("expected 1 raw arg, got %d", len(clause.RawArgs))
	}
}

func TestRewritePlaceholders(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single placeholder",
			in:   "SELECT * FROM users WHERE id = $1",
			want: "SELECT * FROM users WHERE id = ?",
		},
		{
			name: "multiple placeholders",
			in:   "WHERE a = $1 AND b = $2",
			want: "WHERE a = ? AND b = ?",
		},
		{
			name: "no placeholders",
			in:   "no placeholders",
			want: "no placeholders",
		},
		{
			name: "double-digit placeholder",
			in:   "$1 $2 $10",
			want: "? ? ?",
		},
		{
			name: "placeholder inside single-quoted string",
			in:   "'$1' is not a param",
			want: "'$1' is not a param",
		},
		{
			name: "mixed quoted and unquoted",
			in:   "SELECT * FROM t WHERE col = $1 AND name = '$2 literal' AND x = $2",
			want: "SELECT * FROM t WHERE col = ? AND name = '$2 literal' AND x = ?",
		},
		{
			name: "dollar sign without digit",
			in:   "cost is $USD",
			want: "cost is $USD",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "adjacent placeholders",
			in:   "$1$2$3",
			want: "???",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewritePlaceholders(tt.in)
			if got != tt.want {
				t.Errorf("rewritePlaceholders(%q):\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUsesQuestionPlaceholders(t *testing.T) {
	if postgres.New().UsesQuestionPlaceholders() {
		t.Fatal("postgres must use $N placeholders, not ?")
	}
	if !dialectsqlite.New().UsesQuestionPlaceholders() {
		t.Fatal("sqlite must use ? placeholders")
	}
}

func TestNeedsPlaceholderRewrite(t *testing.T) {
	pg := &Engine{dia: postgres.New()}
	if needsPlaceholderRewrite(pg) {
		t.Fatal("postgres engine must not rewrite placeholders")
	}
	lite := &Engine{dia: dialectsqlite.New()}
	if !needsPlaceholderRewrite(lite) {
		t.Fatal("sqlite engine must rewrite placeholders")
	}
}
