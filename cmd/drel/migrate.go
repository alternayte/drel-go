package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alternayte/drel/internal/codegen"
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/driver/pgxdriver"
	"github.com/alternayte/drel/internal/driver/sqlitedriver"
	"github.com/alternayte/drel/internal/migrate"
)

// openMigrateDriver opens a database driver for migration commands, choosing the
// implementation from the configured dialect (falling back to DSN inspection).
func openMigrateDriver(ctx context.Context, configPath, dsn string) (driver.Driver, error) {
	dialect := ""
	if cfg, err := codegen.LoadConfig(configPath); err == nil {
		dialect = cfg.Dialect
	}
	if dialect == "" {
		if strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, "sqlite://") ||
			dsn == ":memory:" || strings.HasSuffix(dsn, ".db") {
			dialect = "sqlite"
		} else {
			dialect = "postgres"
		}
	}
	if dialect == "sqlite" {
		return sqlitedriver.New(dsn)
	}
	return pgxdriver.New(ctx, dsn)
}

func runMigrate() {
	if len(os.Args) < 3 {
		printMigrateUsage()
		os.Exit(1)
	}

	switch os.Args[2] {
	case "new":
		runMigrateNew()
	case "up":
		runMigrateUp()
	case "down":
		runMigrateDown()
	case "status":
		runMigrateStatus()
	case "lint":
		runMigrateLint()
	default:
		fmt.Fprintf(os.Stderr, "unknown migrate command: %s\n", os.Args[2])
		printMigrateUsage()
		os.Exit(1)
	}
}

func printMigrateUsage() {
	fmt.Fprintln(os.Stderr, "Usage: drel migrate <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  new <name>    Generate a new migration from model definitions")
	fmt.Fprintln(os.Stderr, "  up            Apply all pending migrations")
	fmt.Fprintln(os.Stderr, "  down          Rollback the last applied migration")
	fmt.Fprintln(os.Stderr, "  status        Show migration status")
	fmt.Fprintln(os.Stderr, "  lint          Validate migration file checksums")
}

func cfgPath() string {
	p := "drel.yaml"
	for i := 3; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			p = os.Args[i+1]
			i++
		}
	}
	return p
}

func resolveMigrationsDir(configPath string) string {
	cfg, err := codegen.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate: %v\n", err)
		os.Exit(1)
	}
	dir := cfg.Output.Migrations
	if !filepath.IsAbs(dir) {
		cfgDir, _ := filepath.Abs(filepath.Dir(configPath))
		dir = filepath.Join(cfgDir, dir)
	}
	return dir
}

func requireDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "drel migrate: DATABASE_URL environment variable is required")
		os.Exit(1)
	}
	return dsn
}

func runMigrateNew() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: drel migrate new <name>")
		os.Exit(1)
	}
	name := os.Args[3]
	cp := cfgPath()
	mDir := resolveMigrationsDir(cp)

	cfg, err := codegen.LoadConfig(cp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
		os.Exit(1)
	}

	cfgDir, _ := filepath.Abs(filepath.Dir(cp))
	models, err := codegen.ScanPackages(cfg.Packages, cfgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
		os.Exit(1)
	}
	if len(models) == 0 {
		fmt.Fprintln(os.Stderr, "drel migrate new: no models found")
		os.Exit(1)
	}

	dialect := cfg.Dialect

	// Build the desired logical schema and compare against the persisted snapshot.
	desired := codegen.BuildSchema(models, dialect)
	snapshotPath := filepath.Join(mDir, ".drel_snapshot.json")
	old, hasSnapshot, err := codegen.LoadSnapshot(snapshotPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
		os.Exit(1)
	}

	existing, _ := migrate.ParseMigrationDir(mDir)

	var upSQL, downSQL string
	if !hasSnapshot {
		if len(existing) > 0 {
			// Legacy project adopting snapshots: seed the snapshot from current
			// models without generating a migration.
			if err := codegen.SaveSnapshot(snapshotPath, desired); err != nil {
				fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("drel: initialized schema snapshot from current models (no migration generated); re-run after changing models")
			return
		}
		// First migration: emit the full schema.
		upSQL = codegen.GenerateSchema(models, dialect)
		downSQL = codegen.GenerateDropSchema(models)
	} else {
		// Incremental migration: structured diff of snapshot against desired schema.
		upSQL, downSQL = codegen.DiffSchemas(old, desired, dialect)
		if upSQL == "" && downSQL == "" {
			fmt.Println("drel: no schema changes detected")
			return
		}
	}

	version, err := migrate.WriteMigration(mDir, name, upSQL, downSQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
		os.Exit(1)
	}

	// Persist the snapshot only after the migration is successfully written.
	if err := codegen.SaveSnapshot(snapshotPath, desired); err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("drel: created migration %s_%s\n", version, name)
}

func runMigrateUp() {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(cfgPath())
	ctx := context.Background()

	drv, err := openMigrateDriver(ctx, cfgPath(), dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate up: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir)
	count, err := runner.Up(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate up: %v\n", err)
		os.Exit(1)
	}
	if count == 0 {
		fmt.Println("drel: no pending migrations")
	} else {
		fmt.Printf("drel: applied %d migration(s)\n", count)
	}
}

func runMigrateDown() {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(cfgPath())
	ctx := context.Background()

	drv, err := openMigrateDriver(ctx, cfgPath(), dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate down: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir)
	if err := runner.Down(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate down: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("drel: rolled back last migration")
}

func runMigrateStatus() {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(cfgPath())
	ctx := context.Background()

	drv, err := openMigrateDriver(ctx, cfgPath(), dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate status: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir)
	statuses, err := runner.Status(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate status: %v\n", err)
		os.Exit(1)
	}
	if len(statuses) == 0 {
		fmt.Println("No migrations found")
		return
	}
	for _, s := range statuses {
		var marker string
		switch s.State {
		case migrate.StateApplied:
			marker = "[x]"
		case migrate.StatePending:
			marker = "[ ]"
		case migrate.StateModified:
			marker = "[!]"
		case migrate.StateMissing:
			marker = "[?]"
		default:
			marker = "[ ]"
		}
		label := string(s.State)
		fmt.Printf("  %s  %s_%s  (%s)\n", marker, s.Version, s.Name, label)
	}
}

func runMigrateLint() {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(cfgPath())
	ctx := context.Background()

	drv, err := openMigrateDriver(ctx, cfgPath(), dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate lint: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir)
	issues, err := runner.Lint(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate lint: %v\n", err)
		os.Exit(1)
	}
	if len(issues) == 0 {
		fmt.Println("drel: all migration checksums valid")
		return
	}
	for _, issue := range issues {
		fmt.Fprintf(os.Stderr, "  MODIFIED  %s_%s (checksum mismatch)\n", issue.Version, issue.Name)
	}
	os.Exit(1)
}
