package codegen

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SaveSnapshot writes the schema to path as pretty-printed JSON, creating the
// parent directory if needed. The file is written with 0644 permissions.
func SaveSnapshot(path string, s Schema) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// LoadSnapshot reads a schema snapshot from path. The returned bool is false
// when the file does not exist (which is not treated as an error).
func LoadSnapshot(path string) (Schema, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Schema{}, false, nil
		}
		return Schema{}, false, err
	}
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return Schema{}, false, err
	}
	return s, true, nil
}
