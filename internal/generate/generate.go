// Package generate implements SpecGuardian source generation.
package generate

import (
	"context"
	"path/filepath"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/mrasu/spec-guardian/internal/generate/renderer"
)

// Run resolves inputs, extracts cases, renders files, and replaces outputs.
func Run(ctx context.Context, options Options) error {
	config, err := NewConfig(options)
	if err != nil {
		return err
	}
	project, err := newProject(config.projectDir)
	if err != nil {
		return err
	}
	model, err := fizzbee.ReadModel(config.fizzbeeOutputDir)
	if err != nil {
		return err
	}
	declarations, err := resolveDeclarations(config, model.Cases)
	if err != nil {
		return err
	}
	scaffolds, err := renderer.GenerateScaffold(project.modulePath, model, declarations)
	if err != nil {
		return err
	}
	files, err := project.PlanOutputFiles(config.configPath, scaffolds)
	if err != nil {
		return err
	}
	if config.development {
		goMod, err := project.developmentGoMod()
		if err != nil {
			return err
		}
		files[filepath.Join(project.directory, "go.mod")] = goMod
	}
	return replaceAll(files)
}
