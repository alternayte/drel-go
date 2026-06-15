package drel

import "testing"

func TestDoNothingOption_SetsDoNothing(t *testing.T) {
	cfg := &upsertConfig{}
	DoNothing()(cfg)
	if !cfg.doNothing {
		t.Fatal("DoNothing() should set cfg.doNothing = true")
	}
}

func TestDoNothingOption_DropsUpdateRequirement(t *testing.T) {
	// In DoNothing mode, UpdateOnConflict must be optional: a config with a
	// conflict target, no update columns, and doNothing=true must be valid.
	cfg := &upsertConfig{}
	ConflictColumns(NewStringCol("email"))(cfg)
	DoNothing()(cfg)

	if len(cfg.conflictCols) != 1 || cfg.conflictCols[0] != "email" {
		t.Fatalf("expected conflict col email, got %v", cfg.conflictCols)
	}
	if !cfg.doNothing {
		t.Fatal("expected doNothing=true")
	}
	if len(cfg.updateCols) != 0 {
		t.Fatalf("expected no update cols, got %v", cfg.updateCols)
	}
}
