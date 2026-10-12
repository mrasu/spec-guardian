package sql

import (
	"database/sql"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/observer"
	"github.com/stretchr/testify/require"
)

func TestNewObserver_RejectsInvalidSQLConfiguration(t *testing.T) {
	validField := observer.FieldRequest{
		Field: "DB.records",
		Config: map[string]any{
			"query": "SELECT key, value FROM records",
			"result": map[string]any{
				"type":         "kv",
				"key_column":   "key",
				"value_column": "value",
			},
		},
		Output: observer.OutputSpec{Type: observer.Type{Kind: observer.KindMap, Values: &observer.Type{Kind: observer.KindString}}},
	}
	tests := []struct {
		name   string
		config map[string]any
		field  observer.FieldRequest
		want   string
	}{
		{"missing query", nil, withoutFieldConfig(validField, "query"), "requires query"},
		{"missing result", nil, withoutFieldConfig(validField, "result"), "requires result"},
		{"unknown result type", nil, withResultConfig(validField, "type", "rows"), "unsupported result.type"},
		{"missing column", nil, withoutResultConfig(validField, "key_column"), "requires key_column and value_column"},
		{"wrong output", nil, observer.FieldRequest{Field: "DB.records", Config: validField.Config, Output: observer.OutputSpec{Type: observer.Type{Kind: observer.KindString}}}, "requires map of strings"},
		{"wrong connection type", map[string]any{"connection_string": 123}, validField, "connection_string must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := NewObserver(Config{Database: &sql.DB{}, Driver: "pgx"}, tt.config, []observer.FieldRequest{tt.field})
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, instance)
		})
	}
}

func withoutResultConfig(field observer.FieldRequest, name string) observer.FieldRequest {
	field, result := copyResultConfig(field)
	delete(result, name)
	return field
}

func withResultConfig(field observer.FieldRequest, name string, value any) observer.FieldRequest {
	field, result := copyResultConfig(field)
	result[name] = value
	return field
}

func copyResultConfig(field observer.FieldRequest) (observer.FieldRequest, map[string]any) {
	result := make(map[string]any)
	for key, value := range field.Config["result"].(map[string]any) {
		result[key] = value
	}
	field = withoutFieldConfig(field, "result")
	field.Config["result"] = result
	return field, result
}

func TestNewObserver_RequiresDriver(t *testing.T) {
	result, err := NewObserver(Config{Database: &sql.DB{}}, nil, nil)
	require.ErrorContains(t, err, "driver is required")
	require.Nil(t, result)
}

func TestNew_CreatesObserver(t *testing.T) {
	result, err := New(Config{Database: &sql.DB{}, Driver: "pgx"}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func withoutFieldConfig(field observer.FieldRequest, name string) observer.FieldRequest {
	config := make(map[string]any, len(field.Config))
	for key, value := range field.Config {
		if key != name {
			config[key] = value
		}
	}
	field.Config = config
	return field
}
