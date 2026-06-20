package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const version = "0.1.0"

// Run executes the CLI and returns an exit code.
func Run(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("brain", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version")
	name := fs.String("name", "world", "name to greet")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintf(stdout, "brain %s\n", version)
		return 0
	}

	fmt.Fprintf(stdout, "hello, %s\n", *name)
	return 0
}


