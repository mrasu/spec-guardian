package cmd

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/internal/initialize"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var outputDir string
	command := &cobra.Command{
		Use:   "init --fizzbee-output-dir DIR",
		Short: "Set up SpecGuardian for a Go project.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if outputDir == "" {
				return fmt.Errorf("--fizzbee-output-dir is required")
			}
			projectDir, err := os.Getwd()
			if err != nil {
				return errors.Wrap(err, "read working directory")
			}
			return initialize.Run(projectDir, outputDir)
		},
	}
	command.Flags().StringVar(&outputDir, "fizzbee-output-dir", "", "Directory containing FizzBee exploration results")
	return command
}
