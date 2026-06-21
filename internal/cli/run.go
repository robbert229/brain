package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/robbert229/brain/internal/tncli"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

// Run executes the CLI and returns an exit code.
func Run(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, stdout, stderr io.Writer) int {
	var showVersion bool
	var name string

	cmd := &cobra.Command{
		Use:          "brain",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if showVersion {
				_, err := fmt.Fprintf(stdout, "brain %s\n", version)
				return err
			}

			_, err := fmt.Fprintf(stdout, "hello, %s\n", name)
			return err
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().BoolVar(&showVersion, "version", false, "print version")
	cmd.Flags().StringVar(&name, "name", "world", "name to greet")
	cmd.AddCommand(tncli.NewCommand(stdout))
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return 2
	}
	return 0
}
