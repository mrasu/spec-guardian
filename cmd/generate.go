package cmd

import (
	"fmt"

	"github.com/mrasu/spec-guardian/internal/generate"
	"github.com/spf13/cobra"
)

func newGenerateCommand() *cobra.Command {
	var projectDir, fizzOutputDir string
	var dev bool
	command := &cobra.Command{
		Use:   "generate --fizz-output-dir DIR",
		Short: "Generate SpecGuardian files for a Go project.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if fizzOutputDir == "" {
				return fmt.Errorf("--fizz-output-dir is required")
			}

			return generate.Run(command.Context(), generate.Options{ProjectDir: projectDir, FizzOutputDir: fizzOutputDir, Development: dev})
		},
	}
	command.Flags().StringVar(&projectDir, "project-dir", ".", "project directory")
	command.Flags().StringVar(&fizzOutputDir, "fizz-output-dir", "", "FizzBee artifact directory")
	command.Flags().BoolVar(&dev, "dev", false, "use for the local development")
	return command
}
