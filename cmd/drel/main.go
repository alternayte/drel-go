package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/alternayte/drel/internal/codegen"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		runInit()
	case "generate":
		runGenerate()
	case "migrate":
		runMigrate()
	case "seed":
		runSeed()
	case "version":
		fmt.Printf("drel %s (%s)\n", version, commit)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
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

func runInit() {
	configPath := "drel.yaml"
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
		}
	}

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

func runSeed() {
	configPath := "drel.yaml"
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
		}
	}

	cfg, err := codegen.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drel seed: %v\n", err)
		os.Exit(1)
	}
	if cfg.Seed == "" {
		fmt.Fprintln(os.Stderr, "drel seed: no `seed:` package configured in drel.yaml")
		fmt.Fprintln(os.Stderr, "  Add e.g. `seed: ./db/seed` pointing at a Go main that seeds the database.")
		os.Exit(1)
	}

	cmd := exec.Command("go", "run", cfg.Seed)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "drel seed: %v\n", err)
		os.Exit(1)
	}
}

func runGenerate() {
	configPath := "drel.yaml"
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
		}
	}

	if err := codegen.Generate(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "drel generate: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("drel: code generation complete")
}
