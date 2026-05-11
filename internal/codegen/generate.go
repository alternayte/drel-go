package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Generate runs the full code generation pipeline: loads config, scans packages,
// emits per-model files, and emits the aggregated DB file.
func Generate(configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	// Resolve the config file's directory as the working directory for scanning.
	cfgDir := filepath.Dir(configPath)
	if !filepath.IsAbs(cfgDir) {
		abs, err := filepath.Abs(cfgDir)
		if err != nil {
			return fmt.Errorf("codegen: resolve config dir: %w", err)
		}
		cfgDir = abs
	}

	models, err := ScanPackages(cfg.Packages, cfgDir)
	if err != nil {
		return err
	}

	if len(models) == 0 {
		return fmt.Errorf("codegen: no models found in packages %v", cfg.Packages)
	}

	// Emit per-model files.
	for _, m := range models {
		content, err := EmitModelFileChecked(m)
		if err != nil {
			return err
		}
		outPath := filepath.Join(m.Dir, strings.ToLower(m.Name)+"_drel.go")
		if err := writeFile(outPath, content); err != nil {
			return fmt.Errorf("codegen: write model file %s: %w", outPath, err)
		}
	}

	// Emit the aggregated DB file.
	dbPath := cfg.Output.DB
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfgDir, dbPath)
	}
	dbPkgName := filepath.Base(filepath.Dir(dbPath))
	content := EmitDBFile(models, dbPkgName)
	if err := writeFile(dbPath, content); err != nil {
		return fmt.Errorf("codegen: write db file %s: %w", dbPath, err)
	}

	return nil
}

func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}
