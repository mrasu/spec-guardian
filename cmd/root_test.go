package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute_RejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"unknown"}, want: "unknown command"},
		{name: "missing init output", args: []string{"init"}, want: "--fizzbee-output-dir is required"},
		{name: "init positional argument", args: []string{"init", "extra"}, want: "unknown command"},
		{name: "missing config", args: []string{"generate"}, want: "--config is required"},
		{name: "positional argument", args: []string{"generate", "extra", "--config", "specguardian.yml"}, want: "unknown command"},
		{name: "unknown flag", args: []string{"generate", "--unknown"}, want: "unknown flag"},
		{name: "removed project flag", args: []string{"generate", "--config", "specguardian.yml", "--project-dir", "."}, want: "unknown flag: --project-dir"},
		{name: "removed output flag", args: []string{"generate", "--config", "specguardian.yml", "--fizzbee-output-dir", "artifacts"}, want: "unknown flag: --fizzbee-output-dir"},
		{name: "removed dev flag", args: []string{"generate", "--config", "specguardian.yml", "--dev=false"}, want: "unknown flag: --dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Execute(t.Context(), tt.args)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestExecute_InitCreatesProjectFiles(t *testing.T) {
	outputDir, err := filepath.Abs("../internal/fizzbee/testdata/read_db/fizzbee_output")
	require.NoError(t, err)
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	require.NoError(t, Execute(t.Context(), []string{"init", "--fizzbee-output-dir", outputDir}))
	for _, name := range []string{"specguardian.yml", "conformance/specguardian.go"} {
		contents, err := os.ReadFile(filepath.Join(projectDir, name))
		require.NoError(t, err)
		assert.NotEmpty(t, contents)
	}
}
