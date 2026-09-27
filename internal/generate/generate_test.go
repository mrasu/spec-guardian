package generate

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/mrasu/spec-guardian/internal/fizzbee/artifact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCreatesFixedLayoutAndIsDeterministic(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n\ngo 1.27.0\n"), 0o644))
	fixture, err := filepath.Abs(filepath.Join("testdata", "cache_aside_fizzbee_output"))
	require.NoError(t, err)
	fizzOutputDir := copyFizzBeeOutput(t, fixture)
	require.NoError(t, Run(context.Background(), Options{ProjectDir: project, FizzOutputDir: fizzOutputDir}))
	generated := filepath.Join(project, "conformance", "specguardian", "reader_conformance_generated_test.go")
	first, err := os.ReadFile(generated)
	require.NoError(t, err)
	roleFile := filepath.Join(project, "conformance", "specguardian", "reader_conformance_impl_test.go")
	firstRole, err := os.ReadFile(roleFile)
	require.NoError(t, err)
	require.NoError(t, Run(context.Background(), Options{ProjectDir: project, FizzOutputDir: fizzOutputDir}))
	second, err := os.ReadFile(generated)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	secondRole, err := os.ReadFile(roleFile)
	require.NoError(t, err)
	assert.Equal(t, firstRole, secondRole)
	assert.True(t, bytes.HasPrefix(secondRole, []byte("//go:build specguardian\n\npackage specguardian_test\n\n")))
	for _, name := range []string{
		"conformance/specguardian.go",
		"conformance/specguardian/reader_conformance_cases_generated_test.go",
		"conformance/specguardian/writer_conformance_cases_generated_test.go",
		"conformance/specguardian/reader_conformance_generated_test.go",
		"conformance/specguardian/writer_conformance_generated_test.go",
		"conformance/specguardian/reader_conformance_impl_test.go",
		"conformance/specguardian/writer_conformance_impl_test.go",
		"conformance/specguardian/testdata/reader_read_cases_generated.json",
		"conformance/specguardian/testdata/writer_write_cases_generated.json",
	} {
		_, err := os.Stat(filepath.Join(project, name))
		require.NoError(t, err)
	}
	directive, err := os.ReadFile(filepath.Join(project, "conformance", "specguardian.go"))
	require.NoError(t, err)
	assert.Contains(t, string(directive), `--fizz-output-dir "`+fizzOutputDir+`"`)
	assert.NotContains(t, string(directive), "--fizz-dir")
}

func TestDevelopmentGoModWritesRelativeLocalReplacements(t *testing.T) {
	projectDirectory := t.TempDir()
	goMod := `module example.test/project

go 1.27.0

require github.com/mrasu/spec-guardian v0.0.0 // indirect

replace github.com/mrasu/spec-guardian => /old/spec-guardian

replace github.com/mrasu/spec-guardian/guardian => /old/spec-guardian/guardian
`
	require.NoError(t, os.WriteFile(filepath.Join(projectDirectory, "go.mod"), []byte(goMod), 0o644))
	project, err := newProject(projectDirectory)
	require.NoError(t, err)

	updated, err := project.developmentGoMod()
	require.NoError(t, err)
	moduleRoot, err := commandModuleRoot()
	require.NoError(t, err)
	wantRoot, err := filepath.Rel(projectDirectory, moduleRoot)
	require.NoError(t, err)
	wantRoot = localReplacementPath(wantRoot)
	wantGuardian, err := filepath.Rel(projectDirectory, filepath.Join(moduleRoot, "guardian"))
	require.NoError(t, err)
	wantGuardian = localReplacementPath(wantGuardian)
	assert.Contains(t, string(updated), "replace github.com/mrasu/spec-guardian => "+wantRoot)
	assert.Contains(t, string(updated), "replace github.com/mrasu/spec-guardian/guardian => "+wantGuardian)
	assert.Contains(t, string(updated), "github.com/mrasu/spec-guardian v0.0.0 // indirect")
	assert.NotContains(t, string(updated), "/old/spec-guardian")
}

func copyFizzBeeOutput(t *testing.T, source string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "fizzbee_output")
	require.NoError(t, os.MkdirAll(output, 0o755))
	for _, name := range []string{"spec_ast.json", "state_config.json", "nodes_000000_of_000000.pb", "adjacency_lists_000000_of_000000.pb"} {
		contents, err := os.ReadFile(filepath.Join(source, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(output, name), contents, 0o644))
	}
	return output
}

func TestGenerateDoesNotReplaceFilesWhenArtifactReadFails(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n"), 0o644))
	target := filepath.Join(project, "conformance", "model")
	require.NoError(t, os.MkdirAll(target, 0o755))
	file := filepath.Join(target, "guardian_interfaces_generated.go")
	const original = "package model\n\nconst Original = true\n"
	require.NoError(t, os.WriteFile(file, []byte(original), 0o644))

	err := Run(context.Background(), Options{ProjectDir: project, FizzOutputDir: filepath.Join(project, "missing-artifacts")})
	assert.Error(t, err)
	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestGenerateDoesNotReplaceAnyFilesWhenRoleUpdateFails(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n"), 0o644))
	conformance := filepath.Join(project, "conformance", "specguardian")
	require.NoError(t, os.MkdirAll(conformance, 0o755))
	generated := filepath.Join(conformance, "reader_conformance_cases_generated_test.go")
	const original = "package specguardian_test\n\nconst Original = true\n"
	require.NoError(t, os.WriteFile(generated, []byte(original), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(conformance, "reader_conformance_impl_test.go"), []byte("not Go"), 0o644))
	fixture, err := filepath.Abs(filepath.Join("testdata", "cache_aside_fizzbee_output"))
	require.NoError(t, err)
	fizzOutputDir := copyFizzBeeOutput(t, fixture)

	err = Run(context.Background(), Options{ProjectDir: project, FizzOutputDir: fizzOutputDir})
	assert.Error(t, err)
	got, readErr := os.ReadFile(generated)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(got))
}

func TestGenerateKeepsFilesOutsideManagedLayout(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n"), 0o644))
	conformance := filepath.Join(project, "conformance", "specguardian")
	require.NoError(t, os.MkdirAll(conformance, 0o755))
	userFile := filepath.Join(conformance, "helpers_test.go")
	const original = "package specguardian_test\n\nfunc helper() {}\n"
	require.NoError(t, os.WriteFile(userFile, []byte(original), 0o644))
	fixture, err := filepath.Abs(filepath.Join("testdata", "cache_aside_fizzbee_output"))
	require.NoError(t, err)
	fizzOutputDir := copyFizzBeeOutput(t, fixture)

	require.NoError(t, Run(context.Background(), Options{ProjectDir: project, FizzOutputDir: fizzOutputDir}))
	got, err := os.ReadFile(userFile)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestGenerateUpdatesExistingRoleFileAndKeepsUserDeclarations(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n"), 0o644))
	conformance := filepath.Join(project, "conformance", "specguardian")
	require.NoError(t, os.MkdirAll(conformance, 0o755))
	roleFile := filepath.Join(conformance, "reader_conformance_impl_test.go")
	const original = "package specguardian_test\n\nfunc helper() { println(\"kept\") }\n"
	require.NoError(t, os.WriteFile(roleFile, []byte(original), 0o644))
	fixture, err := filepath.Abs(filepath.Join("testdata", "cache_aside_fizzbee_output"))
	require.NoError(t, err)
	fizzOutputDir := copyFizzBeeOutput(t, fixture)

	require.NoError(t, Run(context.Background(), Options{ProjectDir: project, FizzOutputDir: fizzOutputDir}))
	got, err := os.ReadFile(roleFile)
	require.NoError(t, err)
	assert.Contains(t, string(got), `func helper() { println("kept") }`)
	assert.Contains(t, string(got), "func (e *ReaderEnvironment) ObserveCurrentState")
	assert.Contains(t, string(got), "func (e *ReaderEnvironment) BuildAllowedReadState")
}

func TestGenerateRejectsEmptyFizzOutputDir(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n"), 0o644))

	err := Run(context.Background(), Options{ProjectDir: project})
	assert.EqualError(t, err, "FizzBee output directory is empty")
}

func TestGenerateWarnsForArtifactsOutsideProject(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/project\n"), 0o644))
	fixture, err := filepath.Abs(filepath.Join("testdata", "cache_aside_fizzbee_output"))
	require.NoError(t, err)

	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	require.NoError(t, Run(t.Context(), Options{ProjectDir: project, FizzOutputDir: fixture}))
	assert.Contains(t, logs.String(), "generated directive uses an absolute artifact path")
}

func TestExampleFixturesSmoke(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		modulePath string
		roles      []string
	}{
		{
			name:       "cache aside",
			fixture:    filepath.Join("testdata", "cache_aside_fizzbee_output"),
			modulePath: "example.test/cache-aside",
			roles:      []string{"reader", "writer"},
		},
		{
			name:       "idempotent request",
			fixture:    filepath.Join("testdata", "idempotent_request_fizzbee_output"),
			modulePath: "example.test/idempotent-request",
			roles:      []string{"deliveryserver", "orderserver"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := filepath.Abs(test.fixture)
			require.NoError(t, err)
			artifacts, err := artifact.ReadArtifacts(fixture)
			require.NoError(t, err)
			assert.NotEmpty(t, artifacts.Graph.Nodes)
			cases, err := fizzbee.ReadActionCases(fixture)
			require.NoError(t, err)
			assert.NotEmpty(t, cases)
			project := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(project, "go.mod"), []byte("module "+test.modulePath+"\n"), 0o644))

			require.NoError(t, Run(t.Context(), Options{ProjectDir: project, FizzOutputDir: fixture}))
			for _, role := range test.roles {
				for _, suffix := range []string{"_conformance_cases_generated_test.go", "_conformance_generated_test.go", "_conformance_impl_test.go"} {
					parseGeneratedFile(t, filepath.Join(project, "conformance", "specguardian", role+suffix))
				}
			}
			parseGeneratedFile(t, filepath.Join(project, "conformance", "specguardian.go"))
		})
	}
}

func parseGeneratedFile(t *testing.T, name string) {
	t.Helper()
	_, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.AllErrors)
	require.NoError(t, err)
}
