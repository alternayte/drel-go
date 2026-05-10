package main

import (
	"fmt"
	"os"

	"github.com/alternayte/drel/internal/codegen"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		runGenerate()
	case "migrate":
		runMigrate()
	case "version":
		fmt.Println("drel v0.1.0")
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
	fmt.Fprintln(os.Stderr, "  generate    Generate code from model definitions")
	fmt.Fprintln(os.Stderr, "  migrate     Manage database migrations")
	fmt.Fprintln(os.Stderr, "  version     Print version")
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
