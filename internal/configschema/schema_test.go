package configschema

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestValidate_RejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{"numeric version", `{"version":0.1,"fizzbee_files":[{"output_dir":"artifacts"}]}`, "/version"},
		{"unknown kind", `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts","roles":[{"name":"DB","fields":[{"name":"x","type":{"kind":"unknown"}}]}]}]}`, "value must be one of 'string', 'int64'"},
		{"missing exclusion reason", `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts","roles":[{"name":"DB","compare":false}]}]}`, "missing property 'reason'"},
		{"unknown metadata", `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts","sg_metadata":{"unknown":{}}}]}`, "unknown"},
		{"old metadata name", `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts","sg_metadata":{"action_error":{}}}]}`, "action_error"},
		{"invalid crash comparison", `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts","sg_metadata":{"action_crashed":{"compare":"false"}}}]}`, "boolean"},
		{"unknown observer kind", `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts"}],"observers":[{"id":"x","kind":"unknown","plugin":{"builtin":"error_as_crashed"}}]}`, "state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.document), &node))
			err := Validate(&node)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidate_AllowsDedicatedActionCrashedSettings(t *testing.T) {
	tests := []string{
		`{}`,
		`{"action_crashed":{}}`,
		`{"action_crashed":{"compare":true}}`,
		`{"action_crashed":{"compare":false}}`,
	}
	for _, metadata := range tests {
		t.Run(metadata, func(t *testing.T) {
			var node yaml.Node
			document := `{"version":"0.1","fizzbee_files":[{"output_dir":"artifacts","sg_metadata":` + metadata + `}]}`
			require.NoError(t, yaml.Unmarshal([]byte(document), &node))
			require.NoError(t, Validate(&node))
		})
	}
}

func TestValidate_AllowsRecursiveTypesAndScalarObservers(t *testing.T) {
	var node yaml.Node
	err := yaml.Unmarshal([]byte(`{
  "version": "0.1",
  "fizzbee_files": [{"output_dir": "artifacts", "roles": [{"name": "DB", "fields": [
    {"name": "bool", "type": {"kind": "bool", "nullable": true}},
    {"name": "double", "type": {"kind": "double"}},
    {"name": "bytes", "type": {"kind": "bytes", "nullable": true}},
    {"name": "nullable", "type": {"kind": "map", "nullable": true, "values": {"kind": "list", "nullable": true, "values": {"kind": "bool"}}}},
    {"name": "count", "observer": {"id": "db"}, "type": {"kind": "int64"}},
    {"name": "nested", "type": {"kind": "map", "values": {"kind": "list", "compare_order": false,
      "values": {"kind": "message", "fields": {"child": {"kind": "map", "values": {"kind": "int64"}}}}}}}
  ]}]}],
  "observers": [{"id": "db", "plugin": {"builtin": "sql"}, "config": {"custom": [42, true, "text"]}}]
}`), &node)
	require.NoError(t, err)
	err = Validate(&node)
	require.NoError(t, err)
}
