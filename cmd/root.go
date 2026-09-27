// Package cmd implements SpecGuardian command-line operations.
package cmd

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

func newRootCommand() *cobra.Command {
	command := &cobra.Command{Use: "spec-guardian", Short: "Generate conformance tests from FizzBee artifacts.", SilenceErrors: true, SilenceUsage: true}
	command.AddCommand(newGenerateCommand())
	return command
}

// Execute runs the command tree with args.
func Execute(ctx context.Context, args []string) error {
	command := newRootCommand()
	command.SetArgs(args)
	return errors.Wrap(command.ExecuteContext(ctx), "execute command")
}
