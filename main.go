// Command spec-guardian provides the SpecGuardian command-line interface.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mrasu/spec-guardian/cmd"
)

func main() {
	slog.SetDefault(
		slog.New(
			tint.NewTextHandler(
				os.Stderr,
				&tint.Options{
					TimeFormat: time.RFC3339,
				},
			),
		),
	)

	if err := cmd.Execute(context.Background(), os.Args[1:]); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("spec-guardian failed:\n%+v", err))
		os.Exit(1)
	}
}
