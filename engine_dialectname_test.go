package drel_test

import (
	"testing"

	"github.com/alternayte/drel"
)

func TestEngine_DialectName_SQLite(t *testing.T) {
	e, err := drel.NewEngine(":memory:")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(e.Close)
	if got := e.DialectName(); got != "sqlite" {
		t.Fatalf("DialectName() = %q, want %q", got, "sqlite")
	}
}
