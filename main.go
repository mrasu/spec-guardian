// Command spec-guardian provides the SpecGuardian command-line interface.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/mrasu/spec-guardian/cmd"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := cmd.Execute(context.Background(), os.Args[1:]); err != nil {
		slog.ErrorContext(context.Background(), "spec-guardian failed", slog.String("error", fmt.Sprintf("%+v", err)))
		os.Exit(1)
	}
}
