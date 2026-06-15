package dialect_test

import (
	"testing"

	"github.com/alternayte/drel/internal/dialect"
	"github.com/alternayte/drel/internal/dialect/postgres"
	"github.com/alternayte/drel/internal/dialect/sqlite"
)

func TestDialectName(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Dialect
		want string
	}{
		{"postgres", postgres.New(), "postgres"},
		{"sqlite", sqlite.New(), "sqlite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.Name(); got != tc.want {
				t.Fatalf("Name() = %q, want %q", got, tc.want)
			}
		})
	}
}
