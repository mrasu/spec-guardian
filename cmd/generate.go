package cmd

import (
	"fmt"

	"github.com/mrasu/spec-guardian/internal/generate"
	"github.com/spf13/cobra"
)

func newGenerateCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "generate --config FILE",
		Short: "Generate SpecGuardian files for a Go project.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if configPath == "" {
				return fmt.Errorf("--config is required")
			}
			return generate.Run(command.Context(), generate.Options{ConfigPath: configPath})
		},
	}
	command.Flags().StringVar(&configPath, "config", "", "SpecGuardian configuration file")
	return command
}
