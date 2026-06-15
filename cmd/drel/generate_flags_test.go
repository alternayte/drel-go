package main

import "testing"

func TestParseGenerateFlags(t *testing.T) {
	tests := []struct {
		name       string
		argv       []string
		wantConfig string
		wantWatch  bool
	}{
		{"defaults", nil, "drel.yaml", false},
		{"config space", []string{"--config", "x.yaml"}, "x.yaml", false},
		{"config equals", []string{"--config=x.yaml"}, "x.yaml", false},
		{"watch long", []string{"--watch"}, "drel.yaml", true},
		{"watch short", []string{"-w"}, "drel.yaml", true},
		{"watch and config", []string{"--watch", "--config=x.yaml"}, "x.yaml", true},
		{"config short", []string{"-c", "y.yaml"}, "y.yaml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, watch := parseGenerateFlags(tt.argv)
			if cfg != tt.wantConfig {
				t.Fatalf("config = %q, want %q", cfg, tt.wantConfig)
			}
			if watch != tt.wantWatch {
				t.Fatalf("watch = %v, want %v", watch, tt.wantWatch)
			}
		})
	}
}
