package pgxdriver

import (
	"testing"

	"github.com/alternayte/drel/internal/driver"
	"github.com/jackc/pgx/v5"
)

// applyPoolConfig is the pure helper that maps a driver.PoolConfig onto a parsed
// pgxpool config; testing it avoids needing a live Postgres for the exec-mode
// wiring.
func TestApplyPoolConfig_SimpleProtocol(t *testing.T) {
	cfg, err := parseConfigForTest("postgres://u:p@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parseConfigForTest: %v", err)
	}

	applyPoolConfig(cfg, driver.PoolConfig{SimpleProtocol: true})
	if cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Fatalf("DefaultQueryExecMode = %v, want QueryExecModeSimpleProtocol",
			cfg.ConnConfig.DefaultQueryExecMode)
	}

	cfg2, err := parseConfigForTest("postgres://u:p@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parseConfigForTest: %v", err)
	}
	applyPoolConfig(cfg2, driver.PoolConfig{})
	if cfg2.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
		t.Fatalf("default config must not force simple protocol")
	}
}
