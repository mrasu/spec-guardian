// Package generate implements SpecGuardian source generation.
package generate

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/mrasu/spec-guardian/internal/generate/renderer"
)

// Options configures optional generation behavior.
type Options struct {
	ProjectDir    string
	FizzOutputDir string

	// Development records a replace directive to the CLI's development module.
	Development bool
}

// Run resolves inputs, extracts cases, renders files, and replaces outputs.
func Run(ctx context.Context, options Options) error {
	project, err := newProject(options.ProjectDir)
	if err != nil {
		return err
	}
	if options.FizzOutputDir == "" {
		return fmt.Errorf("FizzBee output directory is empty")
	}
	artifactDir, err := filepath.Abs(options.FizzOutputDir)
	if err != nil {
		return errors.Wrap(err, "resolve FizzBee output directory")
	}
	cases, err := fizzbee.ReadActionCases(artifactDir)
	if err != nil {
		return err
	}
	scaffolds, err := renderer.GenerateScaffold(project.modulePath, cases)
	if err != nil {
		return err
	}
	files, outside, err := project.PlanOutputFiles(artifactDir, options.Development, scaffolds)
	if err != nil {
		return err
	}
	if outside {
		slog.WarnContext(ctx, "generated directive uses an absolute artifact path", slog.String("artifact_dir", artifactDir), slog.String("project_dir", project.directory))
	}
	if options.Development {
		goMod, err := project.developmentGoMod()
		if err != nil {
			return err
		}
		files[filepath.Join(project.directory, "go.mod")] = goMod
	}
	return replaceAll(files)
}
