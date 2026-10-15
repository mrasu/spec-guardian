package generate

import (
	"github.com/mrasu/spec-guardian/internal/generate/renderer"
	"os"
	"path/filepath"
	"testing"

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

func TestParseDeclarations_ResultObserverKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := `version: "0.1"
observers:
  - id: action_result
    kind: result
    plugin: {builtin: error_as_crashed}
fizzbee_files:
  - output_dir: artifacts
    roles:
      - name: Application
        fields:
          - name: failed
            observer: {id: action_result}
            type: {kind: bool}
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	config, err := readFileConfig(path)
	require.NoError(t, err)
	declarations, err := parseDeclarations(config)
	require.NoError(t, err)
	require.Len(t, declarations.Observers, 1)
	require.True(t, declarations.Observers[0].Result)
	require.Equal(t, "github.com/mrasu/spec-guardian/guardian/observer/errorascrashed", declarations.Observers[0].PluginImportPath)
}

func TestParseDeclarations_ActionCrashedObserverSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := `version: "0.1"
observers:
  - id: failure
    kind: result
    plugin: {go_package: example.test/statusobserver}
    config: {failure_status_min: 500}
fizzbee_files:
  - output_dir: artifacts
    sg_metadata:
      action_crashed:
        observer:
          id: failure
          config: {status_field: code}
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	config, err := readFileConfig(path)
	require.NoError(t, err)
	declarations, err := parseDeclarations(config)
	require.NoError(t, err)
	request := declarations.ActionCrashedObserver
	require.NotNil(t, request)
	require.True(t, request.Metadata)
	require.True(t, request.Result)
	require.Equal(t, "sg_metadata.action_crashed", request.Field.String())
	require.Equal(t, map[string]any{"failure_status_min": 500}, request.ObserverConfig)
	require.Equal(t, map[string]any{"status_field": "code"}, request.FieldConfig)
}

func TestParseDeclarations_ActionCrashedRequiresResultObserver(t *testing.T) {
	file := fizzbeeFileConfig{SgMetadata: sgMetadataConfig{ActionCrashed: actionCrashedConfig{Observer: &fieldObserverConfig{ID: "db"}}}}
	observers := map[string]resolvedObserver{"db": {Config: observerConfig{ID: "db", Kind: "state"}}}
	declarations, err := buildDeclarations(file, observers)
	require.ErrorContains(t, err, "must have kind result")
	require.Equal(t, renderer.Declarations{}, declarations)
}

func TestValidateObserverConfigs_RejectsIncompatibleKindAndBuiltin(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		builtin string
	}{
		{name: "state plugin as result", kind: "result", builtin: "sql"},
		{name: "result plugin as state", kind: "state", builtin: "error_as_crashed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observers, err := validateObserverConfigs([]observerConfig{{ID: "test", Kind: tt.kind, Plugin: pluginConfig{Builtin: tt.builtin}}})
			require.ErrorContains(t, err, "incompatible")
			require.Nil(t, observers)
		})
	}
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
