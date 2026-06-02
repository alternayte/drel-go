//go:build !libsql

package drel_test

import (
	"errors"
	"testing"

	"github.com/alternayte/drel"
)

// TestNewEngine_LibSQLNotBuilt asserts that without the libsql build tag a
// libSQL DSN fails loudly with a clear, actionable error.
func TestNewEngine_LibSQLNotBuilt(t *testing.T) {
	_, err := drel.NewEngine("libsql://db.turso.io")
	if !errors.Is(err, drel.ErrLibSQLNotBuilt) {
		t.Fatalf("expected ErrLibSQLNotBuilt, got %v", err)
	}
}
