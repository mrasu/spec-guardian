package sql

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	sqlfield "github.com/mrasu/spec-guardian/guardian/provider/sql/internal/field"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestParseConfig_SharedAndFieldSettings(t *testing.T) {
	tests := []struct {
		name   string
		shared string
		field  string
		valid  bool
	}{
		{name: "omitted settings", shared: `{}`, field: ``, valid: true},
		{name: "table and column mapping", shared: `tables: [{name: records, columns: [{name: stored, type: bytes}]}]`, field: `table: records
columns: [{field: value, column: stored}]`, valid: true},
		{name: "kv query", shared: `{}`, field: `query: SELECT key, value FROM records
result: {type: kv, key_column: key, value_column: value}`, valid: true},
		{name: "row map query", shared: `{}`, field: `query: SELECT key, value FROM records
result: {type: row_map, key_column: key}`, valid: true},
		{name: "unknown shared setting", shared: `drvier: pgx`, field: ``, valid: false},
		{name: "invalid column type", shared: `tables: [{name: records, columns: [{name: value, type: map}]}]`, field: ``, valid: false},
		{name: "unknown column setting", shared: `tables: [{name: records, columns: [{name: value, type: string, typo: true}]}]`, field: ``, valid: false},
		{name: "null tables", shared: `tables: null`, field: ``, valid: false},
		{name: "mixed query and table", shared: `{}`, field: `table: records
query: SELECT value FROM records
result: {type: row_map, key_column: value}`, valid: false},
		{name: "incomplete mapping", shared: `{}`, field: `table: records
columns: [{field: value}]`, valid: false},
		{name: "unknown field setting", shared: `{}`, field: `table: records
typo: true`, valid: false},
		{name: "unknown result setting", shared: `{}`, field: `query: SELECT key, value FROM records
result: {type: row_map, key_column: key, typo: true}`, valid: false},
		{name: "non-list tables", shared: `tables: records`, field: ``, valid: false},
		{name: "non-object table", shared: `tables: [records]`, field: ``, valid: false},
		{name: "numeric table name", shared: `tables: [{name: 123, columns: [{name: key, type: string}]}]`, field: ``, valid: false},
		{name: "missing table name", shared: `tables: [{columns: [{name: key, type: string}]}]`, field: ``, valid: false},
		{name: "missing columns", shared: `tables: [{name: records}]`, field: ``, valid: false},
		{name: "empty columns", shared: `tables: [{name: records, columns: []}]`, field: ``, valid: false},
		{name: "non-object column", shared: `tables: [{name: records, columns: [key]}]`, field: ``, valid: false},
		{name: "numeric column name", shared: `tables: [{name: records, columns: [{name: 123, type: string}]}]`, field: ``, valid: false},
		{name: "numeric column type", shared: `tables: [{name: records, columns: [{name: key, type: 123}]}]`, field: ``, valid: false},
		{name: "missing column name", shared: `tables: [{name: records, columns: [{type: string}]}]`, field: ``, valid: false},
		{name: "query result on table", shared: `{}`, field: `table: records
result: {}`, valid: false},
		{name: "non-list mappings", shared: `{}`, field: `table: records
columns: key`, valid: false},
		{name: "null mappings", shared: `{}`, field: `table: records
columns: null`, valid: false},
		{name: "non-object mapping", shared: `{}`, field: `table: records
columns: [key]`, valid: false},
		{name: "numeric mapping field", shared: `{}`, field: `table: records
columns: [{field: 123, column: key}]`, valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(tt.shared), &config))
			var fields []provider.FieldRequest
			if tt.field != "" {
				var settings map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(tt.field), &settings))
				fields = []provider.FieldRequest{{Field: "role.records", Config: settings}}
			}
			parsed, err := parseConfig(config, fields)
			if tt.valid {
				require.NoError(t, err)
				require.NotNil(t, parsed)
			} else {
				require.Error(t, err)
				require.Nil(t, parsed)
			}
		})
	}
}

func TestNewProvider_RejectsUnknownConfigBeforeConnecting(t *testing.T) {
	instance, err := NewProvider(Config{}, map[string]any{"typo": true}, nil)
	require.ErrorContains(t, err, "additional properties 'typo' not allowed")
	require.Nil(t, instance)
}

func TestNewProvider_RejectsInvalidFieldConfigBeforeConnecting(t *testing.T) {
	instance, err := NewProvider(Config{}, nil, []provider.FieldRequest{{
		Field:  "role.records",
		Config: map[string]any{"typo": true},
	}})
	require.ErrorContains(t, err, "field role.records config")
	require.ErrorContains(t, err, "additional properties")
	require.Nil(t, instance)
}

func TestParseConfig_DecodesIndependentSettings(t *testing.T) {
	column := map[string]any{"name": "stored", "type": "bytes"}
	mapping := map[string]any{"field": "value", "column": "stored"}
	config := map[string]any{
		"driver": "pgx", "connection_string": "postgres://localhost/test",
		"tables": []map[string]any{{"name": "records", "columns": []map[string]any{column}}},
	}
	output := provider.OutputSpec{Type: provider.Type{Kind: provider.KindString}}
	fields := []provider.FieldRequest{
		{Field: "DB.rows", Config: map[string]any{"table": "records", "columns": []map[string]any{mapping}}, Output: output},
		{Field: "DB.kv", Config: map[string]any{"query": "SELECT key, value FROM records", "result": map[string]any{"type": "kv", "key_column": "key", "value_column": "value"}}, Output: output},
		{Field: "DB.messages", Config: map[string]any{"query": "SELECT key, value FROM records", "result": map[string]any{"type": "row_map", "key_column": "key"}}, Output: output},
	}
	parsed, err := parseConfig(config, fields)
	require.NoError(t, err)

	config["driver"] = "mysql"
	column["name"] = "changed"
	mapping["column"] = "changed"
	fields[1].Config["result"].(map[string]any)["key_column"] = "changed"
	fields[2].Field = "changed"

	require.Equal(t, &providerConfig{
		Driver: "pgx", ConnectionString: "postgres://localhost/test",
		Tables: []sqlfield.Table{{Name: "records", Columns: []sqlfield.Column{{Name: "stored", Kind: provider.KindBytes}}}},
		Fields: []sqlfield.Request{
			{Field: "DB.rows", Config: sqlfield.Config{Table: "records", Columns: []sqlfield.ColumnMapping{{Field: "value", Column: "stored"}}}, Output: output},
			{Field: "DB.kv", Config: sqlfield.Config{Query: "SELECT key, value FROM records", Result: sqlfield.ResultConfig{Type: sqlfield.ResultTypeKV, KeyColumn: "key", ValueColumn: "value"}}, Output: output},
			{Field: "DB.messages", Config: sqlfield.Config{Query: "SELECT key, value FROM records", Result: sqlfield.ResultConfig{Type: sqlfield.ResultTypeRowMap, KeyColumn: "key"}}, Output: output},
		},
	}, parsed)
}
