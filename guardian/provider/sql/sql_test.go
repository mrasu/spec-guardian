package sql

import (
	"database/sql"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_RejectsInvalidSQLConfiguration(t *testing.T) {
	validField := provider.FieldRequest{
		Field: "DB.records",
		Config: map[string]any{
			"query": "SELECT key, value FROM records",
			"result": map[string]any{
				"type":         "kv",
				"key_column":   "key",
				"value_column": "value",
			},
		},
		Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString}}},
	}
	tests := []struct {
		name   string
		config map[string]any
		field  provider.FieldRequest
		want   string
	}{
		{"missing query", nil, withoutFieldConfig(validField, "query"), "missing property 'query'"},
		{"missing result", nil, withoutFieldConfig(validField, "result"), "missing property 'result'"},
		{"unknown result type", nil, withResultConfig(validField, "type", "rows"), "/result/type"},
		{"missing column", nil, withoutResultConfig(validField, "key_column"), "missing property 'key_column'"},
		{"wrong output", nil, provider.FieldRequest{Field: "DB.records", Config: validField.Config, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindString}}}, "requires map of strings"},
		{"wrong connection type", map[string]any{"connection_string": 123}, validField, "/connection_string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := NewProvider(Config{Database: &sql.DB{}, Driver: "pgx"}, tt.config, []provider.FieldRequest{tt.field})
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, instance)
		})
	}
}

func withoutResultConfig(field provider.FieldRequest, name string) provider.FieldRequest {
	field, result := copyResultConfig(field)
	delete(result, name)
	return field
}

func withResultConfig(field provider.FieldRequest, name string, value any) provider.FieldRequest {
	field, result := copyResultConfig(field)
	result[name] = value
	return field
}

func copyResultConfig(field provider.FieldRequest) (provider.FieldRequest, map[string]any) {
	result := make(map[string]any)
	for key, value := range field.Config["result"].(map[string]any) {
		result[key] = value
	}
	field = withoutFieldConfig(field, "result")
	field.Config["result"] = result
	return field, result
}

func TestNewProvider_RequiresDriver(t *testing.T) {
	result, err := NewProvider(Config{Database: &sql.DB{}}, nil, nil)
	require.ErrorContains(t, err, "driver is required")
	require.Nil(t, result)
}

func TestNew_CreatesProvider(t *testing.T) {
	result, err := New(Config{Database: &sql.DB{}, Driver: "pgx"}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func withoutFieldConfig(field provider.FieldRequest, name string) provider.FieldRequest {
	config := make(map[string]any, len(field.Config))
	for key, value := range field.Config {
		if key != name {
			config[key] = value
		}
	}
	field.Config = config
	return field
}

func TestNewProvider_RejectsMultipleTableFieldsForSameTable(t *testing.T) {
	config := map[string]any{"tables": []any{map[string]any{"name": "records", "columns": []any{map[string]any{"name": "value", "type": "string"}}}}}
	shape := provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"value": {Kind: provider.KindString}}}}
	fields := []provider.FieldRequest{
		{Field: "DB.first", Config: map[string]any{"table": "records"}, Output: provider.OutputSpec{Type: shape}},
		{Field: "DB.second", Config: map[string]any{"table": "records"}, Output: provider.OutputSpec{Type: shape}},
	}
	instance, err := NewProvider(Config{Database: &sql.DB{}, Driver: "pgx"}, config, fields)
	require.ErrorContains(t, err, `SQL table "records" is assigned to both "DB.first" and "DB.second"`)
	require.Nil(t, instance)
}
