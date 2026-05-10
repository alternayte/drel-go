package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Migration represents a single versioned migration with up and down SQL.
type Migration struct {
	Version string
	Name    string
	UpSQL   string
	DownSQL string
}

// ParseMigrationDir reads a directory of migration files and returns them
// sorted by version. Files must follow the naming convention:
//
//	<version>_<name>.up.sql
//	<version>_<name>.down.sql
//
// If the directory does not exist, it returns nil with no error.
func ParseMigrationDir(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("migrate: read dir: %w", err)
	}

	upFiles := map[string]string{}
	downFiles := map[string]string{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") {
			key := strings.TrimSuffix(name, ".up.sql")
			content, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			upFiles[key] = string(content)
		} else if strings.HasSuffix(name, ".down.sql") {
			key := strings.TrimSuffix(name, ".down.sql")
			content, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			downFiles[key] = string(content)
		}
	}

	var keys []string
	for k := range upFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var migrations []Migration
	for _, key := range keys {
		parts := strings.SplitN(key, "_", 2)
		version := parts[0]
		name := ""
		if len(parts) > 1 {
			name = parts[1]
		}
		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			UpSQL:   upFiles[key],
			DownSQL: downFiles[key],
		})
	}

	return migrations, nil
}

// WriteMigration creates a new migration file pair in the given directory.
// It returns the generated version string (timestamp in YYYYMMDDHHmmss format).
func WriteMigration(dir, name, upSQL, downSQL string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("migrate: create dir: %w", err)
	}

	version := time.Now().Format("20060102150405")
	slug := strings.ReplaceAll(strings.ToLower(name), " ", "_")

	upPath := filepath.Join(dir, fmt.Sprintf("%s_%s.up.sql", version, slug))
	downPath := filepath.Join(dir, fmt.Sprintf("%s_%s.down.sql", version, slug))

	if err := os.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(downPath, []byte(downSQL), 0644); err != nil {
		return "", err
	}

	return version, nil
}

// ChecksumContent returns the SHA-256 hex digest of the given content string.
func ChecksumContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
