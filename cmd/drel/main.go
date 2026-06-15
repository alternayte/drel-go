package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/alternayte/drel/internal/codegen"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	parsed, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel: %v\n", err)
		printUsage()
		os.Exit(1)
	}

	switch parsed.Command {
	case "init":
		runInit(parsed)
	case "generate":
		runGenerate(parsed)
	case "migrate":
		runMigrate(parsed)
	case "seed":
		runSeed(parsed)
	case "version":
		fmt.Printf("drel %s (%s)\n", version, commit)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", parsed.Command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: drel <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  init        Scaffold a drel.yaml configuration file")
	fmt.Fprintln(os.Stderr, "  generate    Generate code from model definitions")
	fmt.Fprintln(os.Stderr, "  migrate     Manage database migrations")
	fmt.Fprintln(os.Stderr, "  seed        Run seed functions against the database")
	fmt.Fprintln(os.Stderr, "  version     Print version")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --config, -c <path>   Path to drel.yaml (default: drel.yaml)")
}

const defaultConfig = `# drel configuration — see https://github.com/alternayte/drel
packages:
  - ./features/users
  # - ./features/posts

output:
  db: ./db/drel_gen.go          # aggregated DB struct
  migrations: ./db/migrations   # SQL migration files

dialect: postgres               # postgres | sqlite
`

func runInit(parsed parsedCmd) {
	configPath := parsed.ConfigPath

	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(os.Stderr, "drel init: %s already exists; not overwriting\n", configPath)
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "drel init: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("drel: wrote %s\n", configPath)
	fmt.Println("drel: edit the packages list, then run `drel generate`")
}

func runSeed(parsed parsedCmd) {
	cfg, err := codegen.LoadConfig(parsed.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel seed: %v\n", err)
		os.Exit(1)
	}
	if cfg.Seed == "" {
		fmt.Fprintln(os.Stderr, "drel seed: no `seed:` package configured in drel.yaml")
		fmt.Fprintln(os.Stderr, "  Add e.g. `seed: ./db/seed` pointing at a Go main that seeds the database.")
		os.Exit(1)
	}

	ctx, stop := signalContext()
	defer stop()

	cmd := exec.CommandContext(ctx, "go", "run", cfg.Seed)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "drel seed: %v\n", err)
		os.Exit(1)
	}
}

// signalContext returns a context cancelled on SIGINT/SIGTERM plus its stop fn.
// Callers must defer stop() to release the signal handler.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func runGenerate(parsed parsedCmd) {
	if err := codegen.Generate(parsed.ConfigPath); err != nil {
		fmt.Fprintf(os.Stderr, "drel generate: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("drel: code generation complete")
}
