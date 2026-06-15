package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alternayte/drel/internal/codegen"
	"github.com/alternayte/drel/internal/driver"
	"github.com/alternayte/drel/internal/dsn"
	"github.com/alternayte/drel/internal/migrate"
)

// resolveAuthToken reads the Turso/libSQL auth token from the --auth-token flag
// (highest precedence) or the TURSO_AUTH_TOKEN environment variable.
func resolveAuthToken(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--auth-token" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return os.Getenv("TURSO_AUTH_TOKEN")
}

// openMigrateDriver opens a database driver for migration commands. The dialect
// is taken from drel.yaml when present, otherwise inferred from the DSN. LibSQL/
// Turso DSNs (libsql://, wss://, https://, ...) open the libsql driver, with the
// auth token injected from --auth-token / TURSO_AUTH_TOKEN.
func openMigrateDriver(ctx context.Context, configPath, dataSource string) (driver.Driver, error) {
	authToken := resolveAuthToken(os.Args)
	return dsn.OpenDriver(ctx, dataSource, authToken)
}

func runMigrate(parsed parsedCmd) {
	switch parsed.Subcommand {
	case "new":
		runMigrateNew(parsed)
	case "up":
		runMigrateUp(parsed)
	case "down":
		runMigrateDown(parsed)
	case "status":
		runMigrateStatus(parsed)
	case "lint":
		runMigrateLint(parsed)
	case "check":
		runMigrateCheck(parsed)
	default:
		fmt.Fprintf(os.Stderr, "unknown migrate command: %s\n", parsed.Subcommand)
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
	fmt.Fprintln(os.Stderr, "  check         Fail if migration files are not yet applied to the DB")
	fmt.Fprintln(os.Stderr, "                NOTE: compares file list vs drel_migrations table only;")
	fmt.Fprintln(os.Stderr, "                does not detect out-of-band schema changes (manual ALTERs).")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --config <path>      Path to drel.yaml (default ./drel.yaml)")
	fmt.Fprintln(os.Stderr, "  --auth-token <tok>   LibSQL/Turso auth token (or TURSO_AUTH_TOKEN env)")
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
	dataSource := os.Getenv("DATABASE_URL")
	if dataSource == "" {
		fmt.Fprintln(os.Stderr, "drel migrate: DATABASE_URL environment variable is required")
		os.Exit(1)
	}
	return dataSource
}

// runnerDialect resolves the dialect string for the Runner: config dialect when
// set, else DSN inference.
func runnerDialect(configPath, dataSource string) string {
	if cfg, err := codegen.LoadConfig(configPath); err == nil && cfg.Dialect != "" {
		// drel.yaml uses "sqlite" for libsql; re-detect so libsql is distinguished
		// for lock selection.
		if d := dsn.DetectDialect(dataSource); d == "libsql" {
			return "libsql"
		}
		return cfg.Dialect
	}
	return dsn.DetectDialect(dataSource)
}

func runMigrateNew(parsed parsedCmd) {
	if len(parsed.Positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: drel migrate new <name>")
		os.Exit(1)
	}
	name := parsed.Positional[0]
	cp := parsed.ConfigPath
	mDir := resolveMigrationsDir(cp)

	cfg, err := codegen.LoadConfig(cp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate new: %v\n", err)
		os.Exit(1)
	}

	cfgDir, _ := filepath.Abs(filepath.Dir(cp))
	scanDir := codegen.ResolveModuleRoot(cfgDir)
	models, err := codegen.ScanPackages(cfg.Packages, scanDir)
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
		// First migration: emit the full schema as up. The down is the complete
		// reverse — drop every table (pivots included, in dependency order) and
		// every enum type — derived by diffing the desired schema against an empty
		// one so pivots and enum types are covered (GenerateDropSchema drops only
		// model tables, leaking pivots and enums on rollback).
		upSQL = codegen.GenerateSchema(models, dialect)
		dropUp, _ := codegen.DiffSchemas(desired, codegen.Schema{}, dialect)
		downSQL = dropUp
	} else {
		// Incremental migration: structured diff of snapshot against desired schema.
		upSQL, downSQL = codegen.DiffSchemas(old, desired, dialect)
		if upSQL == "" && downSQL == "" {
			fmt.Println("drel: no schema changes detected")
			return
		}
	}

	if len(existing) > 0 {
		fmt.Fprintln(os.Stderr, "drel: tip: run `drel migrate check` to confirm all prior migrations are applied before deploying")
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

func runMigrateUp(parsed parsedCmd) {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(parsed.ConfigPath)
	ctx, stop := signalContext()
	defer stop()

	drv, err := openMigrateDriver(ctx, parsed.ConfigPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate up: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir, runnerDialect(parsed.ConfigPath, dsn))
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

func runMigrateDown(parsed parsedCmd) {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(parsed.ConfigPath)
	ctx, stop := signalContext()
	defer stop()

	drv, err := openMigrateDriver(ctx, parsed.ConfigPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate down: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir, runnerDialect(parsed.ConfigPath, dsn))
	if err := runner.Down(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate down: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("drel: rolled back last migration")
}

func runMigrateStatus(parsed parsedCmd) {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(parsed.ConfigPath)
	ctx, stop := signalContext()
	defer stop()

	drv, err := openMigrateDriver(ctx, parsed.ConfigPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate status: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir, runnerDialect(parsed.ConfigPath, dsn))
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

func runMigrateLint(parsed parsedCmd) {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(parsed.ConfigPath)
	ctx, stop := signalContext()
	defer stop()

	drv, err := openMigrateDriver(ctx, parsed.ConfigPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate lint: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir, runnerDialect(parsed.ConfigPath, dsn))
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

func runMigrateCheck(parsed parsedCmd) {
	dsn := requireDSN()
	mDir := resolveMigrationsDir(parsed.ConfigPath)
	ctx, stop := signalContext()
	defer stop()

	drv, err := openMigrateDriver(ctx, parsed.ConfigPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate check: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close()

	runner := migrate.NewRunner(drv, mDir, runnerDialect(parsed.ConfigPath, dsn))
	pending, err := runner.Pending(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel migrate check: %v\n", err)
		os.Exit(1)
	}
	if len(pending) == 0 {
		// NOTE: "no unapplied files" is not the same as "no schema drift" —
		// manual out-of-band ALTERs are not detected here. Use `migrate lint`
		// to catch checksum tampering; live schema comparison requires an
		// external introspection tool.
		fmt.Println("drel: no unapplied migrations")
		return
	}
	fmt.Fprintf(os.Stderr, "drel: %d unapplied migration(s); run `drel migrate up` before generating new migrations to avoid snapshot drift:\n", len(pending))
	for _, m := range pending {
		fmt.Fprintf(os.Stderr, "  [ ] %s_%s\n", m.Version, m.Name)
	}
	os.Exit(1)
}
