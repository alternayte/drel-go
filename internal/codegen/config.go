package codegen

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Packages []string     `yaml:"packages"`
	Output   OutputConfig `yaml:"output"`
	Dialect  string       `yaml:"dialect"`
	// Seed is an optional path to a Go main package that seeds the database.
	// `drel seed` runs it with `go run`, passing through DATABASE_URL.
	Seed string `yaml:"seed"`
}

type OutputConfig struct {
	DB         string `yaml:"db"`
	Migrations string `yaml:"migrations"`
}

// validDialects is the closed set of code-generation dialects. libSQL is not a
// codegen target; it reuses the SQLite dialect at runtime, so it is excluded here.
var validDialects = map[string]bool{"postgres": true, "sqlite": true}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("codegen: read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("codegen: parse config %s: %w", path, err)
	}
	if len(cfg.Packages) == 0 {
		return nil, fmt.Errorf("codegen: config %s: no packages specified", path)
	}
	if cfg.Output.DB == "" {
		cfg.Output.DB = "./db/drel_gen.go"
	}
	if cfg.Output.Migrations == "" {
		cfg.Output.Migrations = "./db/migrations"
	}
	if cfg.Dialect == "" {
		cfg.Dialect = "postgres"
	}
	if !validDialects[cfg.Dialect] {
		return nil, fmt.Errorf("codegen: config %s: unknown dialect %q (valid: postgres, sqlite)", path, cfg.Dialect)
	}
	return &cfg, nil
}
