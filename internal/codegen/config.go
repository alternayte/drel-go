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
}

type OutputConfig struct {
	DB         string `yaml:"db"`
	Migrations string `yaml:"migrations"`
}

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
	if cfg.Dialect == "" {
		cfg.Dialect = "postgres"
	}
	return &cfg, nil
}
