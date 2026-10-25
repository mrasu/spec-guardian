package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mrasu/spec-guardian/internal/generate/renderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFileConfig_ValidatesBeforeDecoding(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{"numeric version", "version: 0.1\nfizzbee_files: [{output_dir: artifacts}]", "/version"},
		{"numeric role name", "version: '0.1'\nfizzbee_files: [{output_dir: artifacts, roles: [{name: 42}]}]", "/name"},
		{"unknown nested property", "version: '0.1'\nfizzbee_files: [{output_dir: artifacts, roles: [{name: DB, fields: [{name: x, type: {kind: string, extra: true}}]}]}]", "extra"},
		{"duplicate key", "version: '0.1'\nversion: '0.1'\nfizzbee_files: [{output_dir: artifacts}]", "already defined"},
		{"multiple documents", "version: '0.1'\nfizzbee_files: [{output_dir: artifacts}]\n---\nversion: '0.1'", "one YAML document"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "specguardian.yml")
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o644))
			config, err := readFileConfig(path)
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, config)
		})
	}
}

func TestReadFileConfig_ValidatesAllExamples(t *testing.T) {
	tests := []string{"iceberg", "cache_aside", "idempotent_request", "tutorial_cache_aside_write"}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			config, err := readFileConfig(filepath.Join("../../examples", tt, "specguardian.yml"))
			require.NoError(t, err)
			require.Equal(t, "0.1", config.Version)
		})
	}
}

func TestParseDeclarations_ResultProviderCapability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := `version: "0.1"
providers:
  - id: action_result
    source: {builtin: error_as_crashed}
fizzbee_files:
  - output_dir: artifacts
    roles:
      - name: Application
        fields:
          - name: failed
            provider: {id: action_result}
            type: {kind: bool}
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	config, err := readFileConfig(path)
	require.NoError(t, err)
	declarations, err := parseDeclarations(config)
	require.NoError(t, err)
	require.Len(t, declarations.Providers, 1)
	require.True(t, declarations.Providers[0].Result)
	require.Equal(t, "github.com/mrasu/spec-guardian/guardian/provider/errorascrashed", declarations.Providers[0].SourceImportPath)
}

func TestParseDeclarations_ActionCrashedProviderSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := `version: "0.1"
providers:
  - id: failure
    source: {go_package: example.test/statusprovider, capabilities: [observe_result]}
    config: {failure_status_min: 500}
fizzbee_files:
  - output_dir: artifacts
    sg_metadata:
      action_crashed:
        provider:
          id: failure
          config: {status_field: code}
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	config, err := readFileConfig(path)
	require.NoError(t, err)
	declarations, err := parseDeclarations(config)
	require.NoError(t, err)
	request := declarations.ActionCrashedProvider
	require.NotNil(t, request)
	require.True(t, request.Metadata)
	require.True(t, request.Result)
	require.Equal(t, "sg_metadata.action_crashed", request.Field.String())
	require.Equal(t, map[string]any{"failure_status_min": 500}, request.ProviderConfig)
	require.Equal(t, map[string]any{"status_field": "code"}, request.FieldConfig)
}

func TestParseDeclarations_ActionCrashedRequiresResultProvider(t *testing.T) {
	file := fizzbeeFileConfig{SgMetadata: sgMetadataConfig{ActionCrashed: actionCrashedConfig{Provider: &fieldProviderConfig{ID: "db"}}}}
	providers := map[string]resolvedProvider{"db": {Config: providerConfig{ID: "db"}, Source: resolvedSource{Capabilities: []string{"observe"}}}}
	declarations, err := buildDeclarations(file, providers)
	require.ErrorContains(t, err, "must have observe_result capability")
	require.Equal(t, renderer.Declarations{}, declarations)
}

func TestParseDeclarations_PreservesSeedAndObserveCapabilities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := `version: "0.1"
providers:
  - id: shared
    source:
      go_package: example.test/shared
      capabilities: [seed, observe]
fizzbee_files:
  - output_dir: artifacts
    roles:
      - name: DB
        fields:
          - name: ready
            type: {kind: bool}
            provider: {id: shared}
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	config, err := readFileConfig(path)
	require.NoError(t, err)
	declarations, err := parseDeclarations(config)
	require.NoError(t, err)
	require.Len(t, declarations.Providers, 1)
	require.False(t, declarations.Providers[0].Result)
	require.Equal(t, []string{"seed", "observe"}, declarations.Providers[0].Capabilities)
}

func TestParseDeclarations_SeedOnlyProviderCannotObserveField(t *testing.T) {
	field := fieldConfig{Name: "value", Provider: &fieldProviderConfig{ID: "seed"}, Type: typeConfig{Kind: renderer.KindString}}
	name := "DB"
	file := fizzbeeFileConfig{Roles: []roleConfig{{Name: &name, Fields: []fieldConfig{field}}}}
	providers := map[string]resolvedProvider{"seed": {Config: providerConfig{ID: "seed"}, Source: resolvedSource{Capabilities: []string{"seed"}}}}
	declarations, err := buildDeclarations(file, providers)
	require.ErrorContains(t, err, "requires observe or observe_result capability")
	require.Equal(t, renderer.Declarations{}, declarations)
}

func TestReadFileConfig_PreservesActionCrashedComparisonOverride(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name    string
		setting string
		want    *bool
	}{
		{name: "omitted"},
		{name: "enabled", setting: "sg_metadata: {action_crashed: {compare: true}}", want: &trueValue},
		{name: "disabled", setting: "sg_metadata: {action_crashed: {compare: false}}", want: &falseValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "specguardian.yml")
			contents := "version: '0.1'\nfizzbee_files:\n  - output_dir: artifacts\n    " + tt.setting + "\n"
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
			config, err := readFileConfig(path)
			require.NoError(t, err)
			declarations, err := parseDeclarations(config)
			require.NoError(t, err)
			require.Equal(t, tt.want, declarations.CompareActionCrashed)
		})
	}
}

func TestNewConfig_RejectsInvalidConfiguration(t *testing.T) {
	project := t.TempDir()
	configPath := filepath.Join(project, "specguardian.yml")
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "unsupported version", contents: "version: '0.2'\nfizzbee_files:\n  - output_dir: artifacts\n", want: "/version"},
		{name: "missing file list", contents: "version: '0.1'\n", want: "fizzbee_files"},
		{name: "empty file list", contents: "version: '0.1'\nfizzbee_files: []\n", want: "fizzbee_files"},
		{name: "empty output directory", contents: "version: '0.1'\nfizzbee_files:\n  - output_dir: ''\n", want: "/fizzbee_files/0/output_dir"},
		{name: "multiple files", contents: "version: '0.1'\nfizzbee_files:\n  - output_dir: artifacts\n  - output_dir: another-output\n", want: "maxItems: got 2, want 1"},
		{name: "invalid YAML", contents: "version: [\n", want: "parse configuration file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(configPath, []byte(tt.contents), 0o644))
			config, err := NewConfig(Options{ConfigPath: configPath})
			assert.ErrorContains(t, err, tt.want)
			assert.Nil(t, config)
		})
	}
}

func TestNewConfig_ResolvesArtifactDirectory(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "specguardian.yml")
	contents := "version: '0.1'\nfizzbee_files:\n  - output_dir: docs/spec/fizzbee_output\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))

	config, err := NewConfig(Options{ConfigPath: path})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(project, "docs/spec/fizzbee_output"), config.fizzbeeOutputDir)

}

func TestNewConfig_RejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := "version: '0.1'\nfizzbee_files:\n  - output_dir: artifacts\nunknown_setting: true\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))

	config, err := NewConfig(Options{ConfigPath: path})
	require.ErrorContains(t, err, "additional properties 'unknown_setting' not allowed")
	require.Nil(t, config)
}

func TestNewConfig_ResolvesUserHooks(t *testing.T) {
	tests := []struct {
		name      string
		setting   string
		want      renderer.Hooks
		wantError string
	}{
		{name: "omitted"},
		{name: "empty", setting: "hooks: []"},
		{name: "cleanup only", setting: "hooks: [cleanup_case]", want: renderer.Hooks{CleanupCase: true}},
		{name: "all", setting: "hooks: [setup_case, cleanup_case, setup_attempt, cleanup_attempt]", want: renderer.Hooks{SetupCase: true, CleanupCase: true, SetupAttempt: true, CleanupAttempt: true}},
		{name: "unknown", setting: "hooks: [unknown]", wantError: "/hooks/0"},
		{name: "duplicate", setting: "hooks: [cleanup_case, cleanup_case]", wantError: "are equal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "specguardian.yml")
			contents := "version: '0.1'\nfizzbee_files:\n  - output_dir: artifacts\n    " + tt.setting + "\n"
			require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
			config, err := NewConfig(Options{ConfigPath: path})
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				require.Nil(t, config)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, config.declarations.Hooks)
		})
	}
}
