package drel

import "testing"

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
