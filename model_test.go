package drel

import "testing"

func TestModelSetID(t *testing.T) {
	type widget struct{ Model[string] }
	w := &widget{}
	w.SetID("abc")
	if w.ID() != "abc" {
		t.Fatalf("expected id abc, got %q", w.ID())
	}
}
