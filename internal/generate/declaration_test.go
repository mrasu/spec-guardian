package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/mrasu/spec-guardian/internal/generate/renderer"
	"github.com/stretchr/testify/require"
)

func TestResolveDeclarationsUsesProviderID(t *testing.T) {
	resolved, cases := observationFixture(t)
	declarations, err := resolveDeclarations(resolved, cases)
	require.NoError(t, err)
	providers := declarations.Providers
	require.Len(t, providers, 1)
	require.Equal(t, "DB.records", providers[0].Field.String())
	require.Equal(t, "db", providers[0].ProviderID)
	require.Equal(t, "SELECT key, value FROM records", providers[0].FieldConfig["query"])
	require.Equal(t, map[string]any{"type": "kv", "key_column": "key", "value_column": "value"}, providers[0].FieldConfig["result"])
}

func TestTypeOnlyFieldNeedsNoProvider(t *testing.T) {
	resolved, cases := observationFixture(t)
	config := strings.Replace(validObservationConfig, "            provider:\n              id: db\n              config:\n                query: SELECT key, value FROM records\n                result:\n                  type: kv\n                  key_column: key\n                  value_column: value\n", "", 1)
	require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
	resolved, err := NewConfig(Options{ConfigPath: resolved.configPath})
	require.NoError(t, err)
	require.Empty(t, resolved.declarations.Providers)
	require.Len(t, resolved.declarations.TypedFields, 1)
	require.Equal(t, "DB.records", resolved.declarations.TypedFields[0].Field.String())
	_, err = resolveDeclarations(resolved, cases)
	require.NoError(t, err)
}

func TestValidateFizzBeeValue_Int64(t *testing.T) {
	spec := renderer.Type{Kind: renderer.KindInt64}
	tests := []struct {
		name  string
		value any
		err   string
	}{
		{name: "integer", value: json.Number("9223372036854775807")},
		{name: "overflow", value: json.Number("9223372036854775808"), err: "invalid int64"},
		{name: "fraction", value: json.Number("1.5"), err: "invalid int64"},
		{name: "float conversion rejected", value: float64(1), err: "want int64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFizzBeeValue(tt.value, &spec)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateFizzBeeValue_NullableMessage(t *testing.T) {
	spec := renderer.Type{Kind: renderer.KindMessage, Nullable: true, Fields: map[string]renderer.Type{
		"sequence_no": {Kind: renderer.KindInt64},
	}}
	tests := []struct {
		name  string
		value any
		err   string
	}{
		{name: "null"},
		{name: "present", value: map[string]any{"sequence_no": json.Number("1")}},
		{name: "missing field", value: map[string]any{}, err: "missing message field"},
		{name: "extra field", value: map[string]any{"sequence_no": json.Number("1"), "extra": true}, err: "extra message field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFizzBeeValue(tt.value, &spec)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolveDeclarationsUsesLoadedConfig(t *testing.T) {
	resolved, cases := observationFixture(t)
	if err := os.WriteFile(resolved.configPath, []byte("version: invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	declarations, err := resolveDeclarations(resolved, cases)
	if err != nil {
		t.Fatal(err)
	}
	require.Len(t, declarations.Providers, 1)
	if got := declarations.Providers[0].ProviderID; got != "db" {
		t.Errorf("DB.records provider = %q, want db", got)
	}
}

func TestNewConfigDecodesExcludedField(t *testing.T) {
	resolved, cases := observationFixture(t)
	config := strings.Replace(validObservationConfig, "providers:\n", "          - name: committed_values\n            compare: false\n            reason: commit history is not stored\nproviders:\n", 1)
	require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
	loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
	require.NoError(t, err)
	require.Equal(t, "DB.committed_values", loaded.declarations.Unobservable[0].Field.String())
	require.Equal(t, "commit history is not stored", loaded.declarations.Unobservable[0].Reason)
	require.Len(t, loaded.declarations.Providers, 1)
	_, err = resolveDeclarations(loaded, cases)
	require.NoError(t, err)
}

func TestNewConfigRejectsInvalidFieldDeclaration(t *testing.T) {
	tests := []struct{ name, replacement, want string }{
		{"missing field name", "          - compare: false\n            reason: unavailable\n", "missing property 'name'"},
		{"missing reason", "          - name: committed_values\n            compare: false\n", "missing property 'reason'"},
		{"blank reason", "          - name: committed_values\n            compare: false\n            reason: '  '\n", "does not match pattern"},
		{"duplicate field", "          - name: records\n            compare: false\n            reason: unavailable\n", "duplicate field"},
		{"duplicate role", "      - name: DB\n", "duplicate role"},
		{"missing role name", "      - fields: []\n", "missing property 'name'"},
		{"empty role name", "      - name: ''\n", "minLength"},
		{"dotted role name", "      - name: DB.extra\n", "invalid role name"},
		{"role missing reason", "      - name: Writer\n        compare: false\n", "missing property 'reason'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, _ := observationFixture(t)
			contents := strings.Replace(validObservationConfig, "providers:\n", tt.replacement+"providers:\n", 1)
			require.NoError(t, os.WriteFile(resolved.configPath, []byte(contents), 0o644))
			loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, loaded)
		})
	}
}

func TestNewConfigRejectsInvalidDeclaration(t *testing.T) {
	for _, test := range []struct {
		name   string
		config string
		want   string
	}{
		{"unknown provider", strings.Replace(validObservationConfig, "id: db\n              config:", "id: missing\n              config:", 1), "unknown provider"},
		{"nested field", strings.Replace(validObservationConfig, "name: records", "name: records.extra", 1), "invalid field path"},
		{"wrong kind", strings.Replace(validObservationConfig, "kind: string", "kind: unknown", 1), "value must be one of 'string', 'int64'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolved, _ := observationFixture(t)
			require.NoError(t, os.WriteFile(resolved.configPath, []byte(test.config), 0o644))
			loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
			require.ErrorContains(t, err, test.want)
			require.Nil(t, loaded)
		})
	}
}

func TestNewConfigAcceptsListProviderField(t *testing.T) {
	resolved, _ := observationFixture(t)
	config := strings.Replace(validObservationConfig, "            type:\n              kind: map\n              values:\n                kind: string", "            type:\n              kind: list\n              values:\n                kind: message\n                fields:\n                  file_path: {kind: string, compare: false, reason: implementation-specific path}\n                  rows:\n                    kind: list\n                    values:\n                      kind: list\n                      values: {kind: string}", 1)
	require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
	loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
	require.NoError(t, err)
	require.Equal(t, renderer.KindList, loaded.declarations.Providers[0].Type.Kind)
	require.True(t, loaded.declarations.Providers[0].Type.Values.Fields["file_path"].SkipComparison)
}

func TestNewConfig_ListComparisonOrder(t *testing.T) {
	tests := []struct {
		name              string
		typeDeclaration   string
		ignoreOrder       bool
		nestedIgnoreOrder bool
		wantErr           string
	}{
		{name: "default order", typeDeclaration: "kind: list\n              values: {kind: string}"},
		{name: "explicit order", typeDeclaration: "kind: list\n              compare_order: true\n              values: {kind: string}"},
		{name: "unordered list", typeDeclaration: "kind: list\n              compare_order: false\n              values: {kind: string}", ignoreOrder: true},
		{name: "nested unordered list", typeDeclaration: "kind: list\n              values:\n                kind: message\n                fields:\n                  rows:\n                    kind: list\n                    compare_order: false\n                    values: {kind: string}", nestedIgnoreOrder: true},
		{name: "map cannot set order", typeDeclaration: "kind: map\n              compare_order: false\n              values: {kind: string}", wantErr: "additional properties 'compare_order'"},
		{name: "scalar cannot set order", typeDeclaration: "kind: list\n              values: {kind: string, compare_order: true}", wantErr: "additional properties 'compare_order'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, _ := observationFixture(t)
			config := strings.Replace(validObservationConfig, "kind: map\n              values:\n                kind: string", tt.typeDeclaration, 1)
			require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
			loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, loaded)
				return
			}
			require.NoError(t, err)
			spec := loaded.declarations.Providers[0].Type
			require.Equal(t, tt.ignoreOrder, spec.IgnoreOrder)
			if spec.Values.Kind == renderer.KindMessage {
				require.Equal(t, tt.nestedIgnoreOrder, spec.Values.Fields["rows"].IgnoreOrder)
			}
		})
	}
}

func TestResolveDeclarationsRejectsInvalidFieldPath(t *testing.T) {
	for _, test := range []struct {
		name, field string
	}{
		{"unknown field", "DB.missing"},
		{"params", "Application.key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolved, cases := observationFixture(t)
			contents := strings.Replace(strings.Replace(validObservationConfig, "name: DB", "name: "+strings.Split(test.field, ".")[0], 1), "name: records", "name: "+strings.Split(test.field, ".")[1], 1)
			require.NoError(t, os.WriteFile(resolved.configPath, []byte(contents), 0o644))
			resolved, err := NewConfig(Options{ConfigPath: resolved.configPath})
			require.NoError(t, err)
			declarations, err := resolveDeclarations(resolved, cases)
			require.ErrorContains(t, err, "invalid field path")
			if declarations.Providers != nil || declarations.Unobservable != nil {
				t.Errorf("resolveDeclarations() = %v, want zero result with error", declarations)
			}
		})
	}
}

func TestNewConfigRejectsInvalidProviderIDs(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries string
		want    string
	}{
		{"duplicate id", "- id: db\n  source: {builtin: sql}\n- id: db\n  source: {builtin: sql}\n", "duplicate provider"},
		{"missing id", "- source: {builtin: sql}\n", "missing property 'id'"},
		{"wrong source", "- id: db\n  source: {builtin: missing}\n", "/source/builtin"},
		{"missing source", "- id: db\n  source: {}\n", "missing property 'builtin'"},
		{"both sources", "- id: db\n  source: {builtin: sql, go_package: example.com/source}\n", "'not' failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolved, _ := observationFixture(t)
			config := validObservationConfig[:strings.Index(validObservationConfig, "providers:\n")] + "providers:\n" + test.entries
			require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
			loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
			require.ErrorContains(t, err, test.want)
			require.Nil(t, loaded)
		})
	}
}

func TestGoPackageProviderGeneratesDirectConstructorCall(t *testing.T) {
	resolved, cases := observationFixture(t)
	config := strings.Replace(validObservationConfig, "source: {builtin: sql}", "source: {go_package: example.com/custom/datafile, capabilities: [observe]}", 1)
	require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
	loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
	require.NoError(t, err)
	sources, err := renderer.GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, loaded.declarations)
	require.NoError(t, err)
	for _, source := range sources.RoleCases {
		require.Contains(t, string(source), "_sgDatafile \"example.com/custom/datafile\"")
		require.Contains(t, string(source), "_sgDatafile.New(")
		require.Contains(t, string(source), "_sgDatafile.Config{}")
		require.Contains(t, string(source), "*_sgDatafile.Provider")
		require.NotContains(t, string(source), " provider.Provider")
	}
}

func TestGoPackageProviderReusesImportAlias(t *testing.T) {
	configs := []providerConfig{
		{ID: "first", Source: sourceConfig{GoPackage: "example.com/custom/provider", Capabilities: []string{"observe"}}},
		{ID: "second", Source: sourceConfig{GoPackage: "example.com/custom/provider", Capabilities: []string{"observe"}}},
		{ID: "third", Source: sourceConfig{GoPackage: "example.com/custom/other", Capabilities: []string{"observe"}}},
		{ID: "builtin", Source: sourceConfig{Builtin: "sql"}},
		{ID: "builtin_path", Source: sourceConfig{GoPackage: builtinSources["sql"].ImportPath, Capabilities: []string{"observe"}}},
	}
	providers, err := validateProviderConfigs(configs)
	require.NoError(t, err)
	require.Equal(t, "_sgProvider", providers["first"].Source.Alias)
	require.Equal(t, providers["first"].Source.Alias, providers["second"].Source.Alias)
	require.Equal(t, "_sgOther", providers["third"].Source.Alias)
	require.Equal(t, providers["builtin"].Source.Alias, providers["builtin_path"].Source.Alias)
}

func TestGoPackageProviderAliasesAvoidCollisions(t *testing.T) {
	configs := []providerConfig{
		{ID: "first", Source: sourceConfig{GoPackage: "example.com/first/metadatafiles", Capabilities: []string{"observe"}}},
		{ID: "numbered", Source: sourceConfig{GoPackage: "example.com/first/metadatafiles1", Capabilities: []string{"observe"}}},
		{ID: "second", Source: sourceConfig{GoPackage: "example.com/second/metadatafiles", Capabilities: []string{"observe"}}},
		{ID: "reused", Source: sourceConfig{GoPackage: "example.com/first/metadatafiles", Capabilities: []string{"observe"}}},
		{ID: "third", Source: sourceConfig{GoPackage: "example.com/third/metadatafiles", Capabilities: []string{"observe"}}},
		{ID: "hyphen", Source: sourceConfig{GoPackage: "example.com/custom/metadata-files", Capabilities: []string{"observe"}}},
		{ID: "underscore", Source: sourceConfig{GoPackage: "example.com/custom/metadata_files", Capabilities: []string{"observe"}}},
	}
	providers, err := validateProviderConfigs(configs)
	require.NoError(t, err)
	got := make(map[string]string, len(providers))
	for id, provider := range providers {
		got[id] = provider.Source.Alias
	}
	require.Equal(t, map[string]string{
		"first":      "_sgMetadatafiles",
		"numbered":   "_sgMetadatafiles1",
		"second":     "_sgMetadatafiles2",
		"reused":     "_sgMetadatafiles",
		"third":      "_sgMetadatafiles3",
		"hyphen":     "_sgMetadataFiles",
		"underscore": "_sgMetadataFiles1",
	}, got)
}

func TestResolveDeclarationsValidatesStringMapValues(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		nullable bool
		wantErr  string
	}{
		{name: "wrong value type", value: true, wantErr: "map key \"bad\": got bool, want string"},
		{name: "null rejected", wantErr: "map key \"bad\": null is not allowed"},
		{name: "nullable string accepts null", nullable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, cases := observationFixture(t)
			config.declarations.Providers[0].Type.Values.Nullable = tt.nullable
			cases[0].Input.Roles[0].Fields["records"] = map[string]any{"bad": tt.value}

			declarations, err := resolveDeclarations(config, cases)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Equal(t, renderer.Declarations{}, declarations)
				return
			}
			require.NoError(t, err)
			require.Equal(t, config.declarations, declarations)
		})
	}
}

func observationFixture(t *testing.T) (*Config, []fizzbee.ActionCase) {
	t.Helper()
	// Keep declaration tests independent of the example's storage model.
	cases, err := fizzbee.NewActionCases([]fizzbee.ActionCase{
		{
			Action: fizzbee.ActionRef{Role: "Application", Ref: 3, Action: "Read"},
			Input: fizzbee.Input{Roles: []fizzbee.RoleState{
				{Name: "DB", Params: fizzbee.JSONObject{}, Fields: fizzbee.JSONObject{
					"records":          map[string]any{"key": "value"},
					"committed_values": map[string]any{"key": []any{"value"}},
				}},
				{Name: "Application", Params: fizzbee.JSONObject{"key": "key"}, Fields: fizzbee.JSONObject{}},
			}},
			Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{
				{Name: "DB", Params: fizzbee.JSONObject{}, Fields: fizzbee.JSONObject{
					"records":          map[string]any{"key": "value"},
					"committed_values": map[string]any{"key": []any{"value"}},
				}},
				{Name: "Application", Params: fizzbee.JSONObject{"key": "key"}, Fields: fizzbee.JSONObject{}},
			}}},
		},
	})
	require.NoError(t, err)
	configPath := filepath.Join(t.TempDir(), "specguardian.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(validObservationConfig), 0o644))
	resolved, err := NewConfig(Options{ConfigPath: configPath})
	require.NoError(t, err)
	return resolved, cases
}

const validObservationConfig = `version: "0.1"
fizzbee_files:
  - output_dir: docs/spec/fizzbee_output
    roles:
      - name: DB
        fields:
          - name: records
            provider:
              id: db
              config:
                query: SELECT key, value FROM records
                result:
                  type: kv
                  key_column: key
                  value_column: value
            type:
              kind: map
              values:
                kind: string
providers:
  - id: db
    source: {builtin: sql}
    config: {connection_string: postgres://example}
`

func TestNewConfig_AcceptsMapWithNestedMessageAndList(t *testing.T) {
	resolved, _ := observationFixture(t)
	config := strings.Replace(validObservationConfig,
		"kind: map\n              values:\n                kind: string",
		"kind: map\n              values:\n                kind: message\n                fields:\n                  snapshot:\n                    kind: message\n                    nullable: true\n                    fields:\n                      id: {kind: int64}\n                  rows:\n                    kind: list\n                    compare_order: false\n                    values: {kind: string}", 1)
	require.NoError(t, os.WriteFile(resolved.configPath, []byte(config), 0o644))
	loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
	require.NoError(t, err)
	spec := loaded.declarations.Providers[0].Type.Values
	require.Equal(t, renderer.KindMessage, spec.Kind)
	require.True(t, spec.Fields["snapshot"].Nullable)
	require.True(t, spec.Fields["rows"].IgnoreOrder)
}

func TestResolveDeclarations_ExcludedRoleKeepsModelInput(t *testing.T) {
	resolved, cases := observationFixture(t)
	contents := `version: "0.1"
fizzbee_files:
  - output_dir: artifacts
    roles:
      - name: DB
        compare: false
        reason: internal model state
        fields:
          - name: records
            compare: false
`
	require.NoError(t, os.WriteFile(resolved.configPath, []byte(contents), 0o644))
	loaded, err := NewConfig(Options{ConfigPath: resolved.configPath})
	require.NoError(t, err)
	declarations, err := resolveDeclarations(loaded, cases)
	require.NoError(t, err)
	require.Empty(t, declarations.Providers)
	require.Empty(t, declarations.TypedFields)
	names := make(map[string]string)
	for _, field := range declarations.Unobservable {
		names[field.Field.FieldName()] = field.Reason
	}
	expected := make(map[string]string)
	for name := range cases[0].Input.Roles[0].Fields {
		expected[name] = "internal model state"
	}
	require.Equal(t, expected, names)
	sources, err := renderer.GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, declarations)
	require.NoError(t, err)
	for _, source := range sources.RoleCases {
		require.Contains(t, string(source), "type ApplicationDBField struct")
		require.Contains(t, string(source), "`json:\"records\"`")
		require.NotContains(t, string(source), "type ApplicationDBObservedField")
		require.NotContains(t, string(source), "type ApplicationDBComparisonField")
	}
	require.NotEmpty(t, cases[0].Input.Roles[0].Fields["records"])
}

func TestNewConfig_ReservesNullRoleForFutureSupport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specguardian.yml")
	contents := `version: "0.1"
fizzbee_files:
  - output_dir: artifacts
    roles:
      - name: null
        fields:
          - name: counter
            type: {kind: int64}
      - name: globals
        fields:
          - name: counter
            type: {kind: int64}
      - name: "null"
        fields:
          - name: counter
            type: {kind: int64}
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	_, err := NewConfig(Options{ConfigPath: path})
	require.ErrorContains(t, err, "got null, want string")
	contents = strings.Replace(contents, "      - name: null\n        fields:\n          - name: counter\n            type: {kind: int64}\n", "", 1)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	config, err := NewConfig(Options{ConfigPath: path})
	require.NoError(t, err)
	require.Len(t, config.declarations.TypedFields, 2)
	require.Equal(t, "globals", config.declarations.TypedFields[0].Field.RoleName())
	require.Equal(t, "null", config.declarations.TypedFields[1].Field.RoleName())
}

func TestNewConfig_ComparisonReasonsAndKindSpecificSettings(t *testing.T) {
	tests := []struct{ name, declaration, want string }{
		{"nested exclusion requires reason", "{kind: map, values: {kind: string, compare: false}}", "missing property 'reason'"},
		{"nested exclusion with reason", "{kind: map, values: {kind: string, compare: false, reason: generated value}}", ""},
		{"reason without exclusion", "{kind: map, reason: model state, values: {kind: string}}", ""},
		{"ignored parent covers children", "{kind: map, values: {kind: message, compare: false, reason: internal state, fields: {child: {kind: string, compare: false}}}}", ""},
		{"scalar rejects values", "{kind: string, values: {kind: string}}", "additional properties 'values' not allowed"},
		{"list rejects fields", "{kind: list, values: {kind: string}, fields: {child: {kind: string}}}", "additional properties 'fields' not allowed"},
		{"message rejects values", "{kind: message, fields: {child: {kind: string}}, values: {kind: string}}", "additional properties 'values' not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "specguardian.yml")
			contents := "version: '0.1'\nfizzbee_files:\n  - output_dir: artifacts\n    roles:\n      - name: DB\n        fields:\n          - name: records\n            type: " + tt.declaration + "\n"
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
			config, err := NewConfig(Options{ConfigPath: path})
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				require.Nil(t, config)
				return
			}
			require.NoError(t, err)
			require.Len(t, config.declarations.TypedFields, 1)
		})
	}
}

func TestNewConfig_AcceptsScalarAndMessageProviderFields(t *testing.T) {
	tests := []struct {
		name, declaration string
		kind              renderer.Kind
		nullable          bool
	}{
		{"string", "kind: string", renderer.KindString, false},
		{"int64", "kind: int64", renderer.KindInt64, false},
		{"nullable string", "kind: string\n              nullable: true", renderer.KindString, true},
		{"nullable int64", "kind: int64\n              nullable: true", renderer.KindInt64, true},
		{"message", "kind: message\n              fields:\n                cnt: {kind: int64}", renderer.KindMessage, false},
		{"nullable message", "kind: message\n              nullable: true\n              fields:\n                cnt: {kind: int64}", renderer.KindMessage, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture, _ := observationFixture(t)
			config := strings.Replace(validObservationConfig, "kind: map\n              values:\n                kind: string", tt.declaration, 1)
			config = strings.Replace(config, "source: {builtin: sql}", "source: {go_package: example.test/custom, capabilities: [observe]}", 1)
			require.NoError(t, os.WriteFile(fixture.configPath, []byte(config), 0600))
			loaded, err := NewConfig(Options{ConfigPath: fixture.configPath})
			require.NoError(t, err)
			require.Len(t, loaded.declarations.Providers, 1)
			spec := loaded.declarations.Providers[0].Type
			require.Equal(t, tt.kind, spec.Kind)
			require.Equal(t, tt.nullable, spec.Nullable)
			require.Nil(t, spec.Values)
		})
	}
}

func TestValidateFizzBeeValue_BasicTypesAndNullableCollections(t *testing.T) {
	tests := []struct {
		name  string
		spec  renderer.Type
		value any
		err   string
	}{
		{"bool", renderer.Type{Kind: renderer.KindBool}, true, ""},
		{"numeric bool", renderer.Type{Kind: renderer.KindBool}, json.Number("1"), "want bool"},
		{"double", renderer.Type{Kind: renderer.KindDouble}, json.Number("1.5"), ""},
		{"integer JSON double", renderer.Type{Kind: renderer.KindDouble}, json.Number("1"), ""},
		{"overflow double", renderer.Type{Kind: renderer.KindDouble}, json.Number("1e400"), "invalid double"},
		{"NaN double", renderer.Type{Kind: renderer.KindDouble}, json.Number("NaN"), "invalid double"},
		{"bytes", renderer.Type{Kind: renderer.KindBytes}, "AP8=", ""},
		{"empty bytes", renderer.Type{Kind: renderer.KindBytes}, "", ""},
		{"invalid base64", renderer.Type{Kind: renderer.KindBytes}, "invalid!", "invalid base64"},
		{"noncanonical base64", renderer.Type{Kind: renderer.KindBytes}, "AB==", "invalid base64"},
		{"integer list bytes", renderer.Type{Kind: renderer.KindBytes}, []any{json.Number("1")}, "want base64 string"},
		{"nullable bytes", renderer.Type{Kind: renderer.KindBytes, Nullable: true}, nil, ""},
		{"nullable map", renderer.Type{Kind: renderer.KindMap, Nullable: true, Values: &renderer.Type{Kind: renderer.KindDouble}}, nil, ""},
		{"empty map", renderer.Type{Kind: renderer.KindMap, Nullable: true, Values: &renderer.Type{Kind: renderer.KindDouble}}, map[string]any{}, ""},
		{"nullable list", renderer.Type{Kind: renderer.KindList, Nullable: true, Values: &renderer.Type{Kind: renderer.KindBool}}, nil, ""},
		{"non-nullable list", renderer.Type{Kind: renderer.KindList, Values: &renderer.Type{Kind: renderer.KindBool}}, nil, "null is not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFizzBeeValue(tt.value, &tt.spec)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewConfig_ReadsExtendedDeclaredTypes(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		want       renderer.Type
	}{
		{"bool", "kind: bool\n                nullable: true", renderer.Type{Kind: renderer.KindBool, Nullable: true}},
		{"double", "kind: double", renderer.Type{Kind: renderer.KindDouble}},
		{"bytes", "kind: bytes\n                nullable: true", renderer.Type{Kind: renderer.KindBytes, Nullable: true}},
		{"nested nullable collections", "kind: map\n                nullable: true\n                values:\n                  kind: list\n                  nullable: true\n                  values: {kind: bool}", renderer.Type{Kind: renderer.KindMap, Nullable: true, Values: &renderer.Type{Kind: renderer.KindList, Nullable: true, Values: &renderer.Type{Kind: renderer.KindBool}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "specguardian.yml")
			config := strings.Replace(validObservationConfig, "kind: string", tt.definition, 1)
			require.NoError(t, os.WriteFile(path, []byte(config), 0600))
			got, err := NewConfig(Options{ConfigPath: path})
			require.NoError(t, err)
			require.Len(t, got.declarations.Providers, 1)
			require.Equal(t, renderer.Type{Kind: renderer.KindMap, Values: &tt.want}, got.declarations.Providers[0].Type)
		})
	}
}
