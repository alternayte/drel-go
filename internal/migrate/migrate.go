package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alternayte/drel/internal/driver"
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

// Runner executes migrations against a database using the driver interface.
type Runner struct {
	drv driver.Driver
	dir string
}

// NewRunner creates a Runner that reads migration files from dir and executes
// them against the provided driver.
func NewRunner(drv driver.Driver, dir string) *Runner {
	return &Runner{drv: drv, dir: dir}
}

func (r *Runner) ensureTable(ctx context.Context) error {
	_, err := r.drv.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS drel_migrations (
			version    VARCHAR(14) PRIMARY KEY,
			name       TEXT NOT NULL,
			checksum   TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("migrate: create tracking table: %w", err)
	}
	return nil
}

func (r *Runner) appliedVersions(ctx context.Context) (map[string]string, error) {
	rows, err := r.drv.Query(ctx, "SELECT version, checksum FROM drel_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("migrate: query applied: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]string)
	for rows.Next() {
		var v, cs string
		if err := rows.Scan(&v, &cs); err != nil {
			return nil, err
		}
		applied[v] = cs
	}
	return applied, rows.Err()
}

// Up applies all pending migrations in version order. Each migration runs in
// its own transaction. Returns the number of migrations applied.
func (r *Runner) Up(ctx context.Context) (int, error) {
	if err := r.ensureTable(ctx); err != nil {
		return 0, err
	}

	migrations, err := ParseMigrationDir(r.dir)
	if err != nil {
		return 0, err
	}

	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}

	// Verify checksums of already-applied migrations before applying new ones.
	for _, m := range migrations {
		storedChecksum, ok := applied[m.Version]
		if !ok {
			continue
		}
		currentChecksum := ChecksumContent(m.UpSQL)
		if currentChecksum != storedChecksum {
			return 0, fmt.Errorf("migration %s has been modified after being applied (expected checksum %s, got %s)",
				m.Version, storedChecksum, currentChecksum)
		}
	}

	count := 0
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}

		tx, err := r.drv.Begin(ctx)
		if err != nil {
			return count, fmt.Errorf("migrate: begin: %w", err)
		}

		if _, err := tx.Exec(ctx, m.UpSQL); err != nil {
			_ = tx.Rollback(ctx)
			return count, fmt.Errorf("migrate: apply %s_%s: %w", m.Version, m.Name, err)
		}

		checksum := ChecksumContent(m.UpSQL)
		if _, err := tx.Exec(ctx,
			"INSERT INTO drel_migrations (version, name, checksum) VALUES ($1, $2, $3)",
			m.Version, m.Name, checksum); err != nil {
			_ = tx.Rollback(ctx)
			return count, fmt.Errorf("migrate: record %s: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return count, fmt.Errorf("migrate: commit %s: %w", m.Version, err)
		}
		count++
	}

	return count, nil
}

// Down rolls back the most recently applied migration.
func (r *Runner) Down(ctx context.Context) error {
	if err := r.ensureTable(ctx); err != nil {
		return err
	}

	migrations, err := ParseMigrationDir(r.dir)
	if err != nil {
		return err
	}

	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return err
	}

	var last *Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		if _, ok := applied[migrations[i].Version]; ok {
			last = &migrations[i]
			break
		}
	}

	if last == nil {
		return fmt.Errorf("migrate: no migrations to roll back")
	}

	if strings.TrimSpace(last.DownSQL) == "" {
		return fmt.Errorf("migrate: migration %s_%s has no down migration (file is empty or missing)", last.Version, last.Name)
	}

	tx, err := r.drv.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: begin: %w", err)
	}

	if _, err := tx.Exec(ctx, last.DownSQL); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("migrate: rollback %s_%s: %w", last.Version, last.Name, err)
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM drel_migrations WHERE version = $1",
		last.Version); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("migrate: unrecord %s: %w", last.Version, err)
	}

	return tx.Commit(ctx)
}

// MigrationState indicates the lifecycle state of a migration.
type MigrationState string

const (
	// StateApplied means the migration is in the directory and has been applied
	// with a matching checksum.
	StateApplied MigrationState = "applied"
	// StatePending means the migration is in the directory but has not been applied.
	StatePending MigrationState = "pending"
	// StateModified means the migration has been applied but the file content
	// no longer matches the stored checksum.
	StateModified MigrationState = "modified"
	// StateMissing means the migration was applied but the file no longer exists
	// in the migration directory.
	StateMissing MigrationState = "missing"
)

// MigrationStatus represents the state of a single migration.
type MigrationStatus struct {
	Version string
	Name    string
	State   MigrationState
	// Applied is true when the migration has been applied to the database,
	// regardless of whether the file still matches. Kept for backward compatibility.
	Applied bool
}

// appliedNames returns a map of version → name for applied migrations.
func (r *Runner) appliedNames(ctx context.Context) (map[string]string, error) {
	rows, err := r.drv.Query(ctx, "SELECT version, name FROM drel_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("migrate: query applied names: %w", err)
	}
	defer rows.Close()

	names := make(map[string]string)
	for rows.Next() {
		var v, n string
		if err := rows.Scan(&v, &n); err != nil {
			return nil, err
		}
		names[v] = n
	}
	return names, rows.Err()
}

// Status returns the status of all migrations, including those in the directory
// and those applied to the database. Each migration is classified as one of:
// applied, pending, modified, or missing.
func (r *Runner) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := r.ensureTable(ctx); err != nil {
		return nil, err
	}

	migrations, err := ParseMigrationDir(r.dir)
	if err != nil {
		return nil, err
	}

	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	appliedN, err := r.appliedNames(ctx)
	if err != nil {
		return nil, err
	}

	// Track which applied versions we've seen in the directory.
	seen := make(map[string]bool)

	var statuses []MigrationStatus
	for _, m := range migrations {
		storedChecksum, isApplied := applied[m.Version]
		seen[m.Version] = true

		var state MigrationState
		if !isApplied {
			state = StatePending
		} else if ChecksumContent(m.UpSQL) != storedChecksum {
			state = StateModified
		} else {
			state = StateApplied
		}

		statuses = append(statuses, MigrationStatus{
			Version: m.Version,
			Name:    m.Name,
			State:   state,
			Applied: isApplied,
		})
	}

	// Append migrations that are applied but no longer exist in the directory.
	for version, checksum := range applied {
		if seen[version] {
			continue
		}
		_ = checksum // checksum not needed for missing entries
		name := appliedN[version]
		statuses = append(statuses, MigrationStatus{
			Version: version,
			Name:    name,
			State:   StateMissing,
			Applied: true,
		})
	}

	// Sort by version for deterministic output.
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Version < statuses[j].Version
	})

	return statuses, nil
}

// LintResult reports a migration file whose content has changed after it was
// applied to the database.
type LintResult struct {
	Version  string
	Name     string
	Expected string
	Actual   string
}

// Lint checks all applied migrations for checksum mismatches, detecting files
// that were modified after being applied.
func (r *Runner) Lint(ctx context.Context) ([]LintResult, error) {
	if err := r.ensureTable(ctx); err != nil {
		return nil, err
	}

	migrations, err := ParseMigrationDir(r.dir)
	if err != nil {
		return nil, err
	}

	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	var issues []LintResult
	for _, m := range migrations {
		storedChecksum, ok := applied[m.Version]
		if !ok {
			continue
		}
		currentChecksum := ChecksumContent(m.UpSQL)
		if currentChecksum != storedChecksum {
			issues = append(issues, LintResult{
				Version:  m.Version,
				Name:     m.Name,
				Expected: storedChecksum,
				Actual:   currentChecksum,
			})
		}
	}

	return issues, nil
}
