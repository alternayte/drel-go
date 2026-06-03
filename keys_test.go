package drel

import (
	"testing"

	"github.com/google/uuid"
)

func TestUUIDv7Key_ReturnsDistinctV7(t *testing.T) {
	a := UUIDv7Key().(uuid.UUID)
	b := UUIDv7Key().(uuid.UUID)
	if a == b {
		t.Fatal("expected distinct UUIDs")
	}
	if a.Version() != 7 {
		t.Fatalf("expected version 7, got %d", a.Version())
	}
}

func TestKeyRegistry_OverrideByTable(t *testing.T) {
	const table = "widgets_test_keys"
	defer clearKeyConfig(table)

	if _, ok := keyConfigFor(table); ok {
		t.Fatal("expected no config before registration")
	}

	setKeyConfig(table, keyConfig{Strategy: KeyAppAssigned, Generate: UUIDv7Key})
	cfg, ok := keyConfigFor(table)
	if !ok || cfg.Strategy != KeyAppAssigned || cfg.Generate == nil {
		t.Fatalf("expected registered app-assigned config, got %+v ok=%v", cfg, ok)
	}
}
