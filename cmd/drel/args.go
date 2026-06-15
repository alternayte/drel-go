package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// parsedCmd is the result of parsing a drel invocation. Handlers consume this
// instead of touching os.Args directly, so the parser is unit-testable without
// os.Exit.
type parsedCmd struct {
	Command    string   // "init" | "generate" | "migrate" | "seed" | "version"
	Subcommand string   // for "migrate": "new"|"up"|"down"|"status"|"lint"; else ""
	ConfigPath string   // resolved value of --config / -c (default "drel.yaml")
	Watch      bool     // generate --watch
	Positional []string // remaining non-flag args (e.g. the migration name)
}

// commandFlags builds a FlagSet for a (sub)command with the shared --config/-c
// flag and, for "generate", a --watch flag. Errors are returned, never printed,
// and the set never calls os.Exit.
func commandFlags(name string, withWatch bool, cfg *string, watch *bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(cfg, "config", "drel.yaml", "path to drel.yaml")
	fs.StringVar(cfg, "c", "drel.yaml", "path to drel.yaml (shorthand)")
	if withWatch {
		fs.BoolVar(watch, "watch", false, "regenerate on file changes")
	}
	return fs
}

// parseArgs parses argv (everything after the program name) into a parsedCmd.
// It uses a stdlib FlagSet per (sub)command so `--config x`, `--config=x`, and
// `-c x` all work, and `migrate new --config x name` yields Positional=["name"]
// rather than the literal "--config". Returns a usage error (never os.Exit).
func parseArgs(argv []string) (parsedCmd, error) {
	if len(argv) == 0 {
		return parsedCmd{}, fmt.Errorf("no command given")
	}

	cmd := argv[0]
	switch cmd {
	case "version":
		return parsedCmd{Command: "version"}, nil

	case "init", "generate", "seed":
		var cfg string
		var watch bool
		fs := commandFlags(cmd, cmd == "generate", &cfg, &watch)
		if err := fs.Parse(argv[1:]); err != nil {
			return parsedCmd{}, fmt.Errorf("%s: %w", cmd, err)
		}
		return parsedCmd{Command: cmd, ConfigPath: cfg, Watch: watch, Positional: fs.Args()}, nil

	case "migrate":
		if len(argv) < 2 {
			return parsedCmd{}, fmt.Errorf("migrate: missing subcommand (new|up|down|status|lint)")
		}
		sub := argv[1]
		switch sub {
		case "new", "up", "down", "status", "lint", "check":
		default:
			return parsedCmd{}, fmt.Errorf("unknown migrate command: %s", sub)
		}
		var cfg string
		var watch bool
		fs := commandFlags("migrate "+sub, false, &cfg, &watch)
		if err := fs.Parse(argv[2:]); err != nil {
			if sub == "new" {
				return parsedCmd{}, fmt.Errorf("migrate new: invalid migration name or flag: %w", err)
			}
			return parsedCmd{}, fmt.Errorf("migrate %s: %w", sub, err)
		}
		pc := parsedCmd{Command: "migrate", Subcommand: sub, ConfigPath: cfg, Positional: fs.Args()}
		if sub == "new" {
			if err := validateMigrationName(pc.Positional); err != nil {
				return parsedCmd{}, err
			}
		}
		return pc, nil

	default:
		return parsedCmd{}, fmt.Errorf("unknown command: %s", cmd)
	}
}

// validateMigrationName ensures `migrate new` got exactly one positional that is
// a sane identifier (no leading dash, no path separators).
func validateMigrationName(positional []string) error {
	if len(positional) == 0 {
		return fmt.Errorf("migrate new: missing migration name")
	}
	name := positional[0]
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("migrate new: invalid migration name %q: must not start with '-' (did a flag value go missing?)", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("migrate new: invalid migration name %q: must not contain path separators", name)
	}
	return nil
}
