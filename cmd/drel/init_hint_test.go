package main

import (
	"strings"
	"testing"
)

func TestInitHint_MentionsGoGenerate(t *testing.T) {
	hint := initGoGenerateHint()
	if !strings.Contains(hint, "//go:generate drel generate") {
		t.Fatalf("init hint must mention the //go:generate directive, got: %q", hint)
	}
	if !strings.Contains(hint, "go generate ./...") {
		t.Fatalf("init hint must mention `go generate ./...`, got: %q", hint)
	}
}

func TestDefaultConfig_DocumentsGoGenerate(t *testing.T) {
	if !strings.Contains(defaultConfig, "//go:generate drel generate") {
		t.Fatalf("defaultConfig should document the //go:generate directive")
	}
}
