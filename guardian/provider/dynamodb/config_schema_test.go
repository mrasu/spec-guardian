package dynamodb

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	dynamofield "github.com/mrasu/spec-guardian/guardian/provider/dynamodb/internal/field"
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
		{name: "dynamic connection", shared: `{}`, field: ``, valid: true},
		{name: "item list", shared: `tables: [{name: records, attribute_definitions: [{attribute_name: id, attribute_type: S}], key_schema: [{attribute_name: id, key_type: HASH}]}]`, field: `table: records
result: {type: item_list}`, valid: true},
		{name: "item map", shared: `endpoint: http://localhost:4566
region: local
access_key_id: test
secret_access_key: test`, field: `table: records
result: {type: item_map}`, valid: true},
		{name: "unknown shared setting", shared: `typo: true`, field: ``, valid: false},
		{name: "invalid endpoint", shared: `endpoint: 123`, field: ``, valid: false},
		{name: "unsupported key type", shared: `tables: [{name: records, attribute_definitions: [{attribute_name: id, attribute_type: N}], key_schema: [{attribute_name: id, key_type: HASH}]}]`, field: ``, valid: false},
		{name: "unknown table setting", shared: `tables: [{name: records, attribute_definitions: [{attribute_name: id, attribute_type: S}], key_schema: [{attribute_name: id, key_type: HASH}], typo: true}]`, field: ``, valid: false},
		{name: "missing result", shared: `{}`, field: `table: records`, valid: false},
		{name: "unknown result type", shared: `{}`, field: `table: records
result: {type: rows}`, valid: false},
		{name: "unknown result setting", shared: `{}`, field: `table: records
result: {type: item_map, typo: true}`, valid: false},
		{name: "unknown field setting", shared: `{}`, field: `table: records
result: {type: item_map}
typo: true`, valid: false},
		{name: "missing table name", shared: `tables: [{attribute_definitions: [{attribute_name: id, attribute_type: S}], key_schema: [{attribute_name: id, key_type: HASH}]}]`, field: ``, valid: false},
		{name: "missing key", shared: `tables: [{name: events, attribute_definitions: [{attribute_name: id, attribute_type: S}]}]`, field: ``, valid: false},
		{name: "sort key", shared: `tables: [{name: events, attribute_definitions: [{attribute_name: id, attribute_type: S}], key_schema: [{attribute_name: id, key_type: RANGE}]}]`, field: ``, valid: false},
		{name: "composite key", shared: `tables: [{name: events, attribute_definitions: [{attribute_name: id, attribute_type: S}], key_schema: [{attribute_name: id, key_type: HASH}, {attribute_name: sort, key_type: RANGE}]}]`, field: ``, valid: false},
		{name: "missing attribute definition", shared: `tables: [{name: events, key_schema: [{attribute_name: id, key_type: HASH}]}]`, field: ``, valid: false},
		{name: "missing table", shared: `{}`, field: `result: {type: item_map}`, valid: false},
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
	config := testProviderConfig("http://localhost:4566", "events")
	fields := []provider.FieldRequest{seedTestField("DynamoDB.events", "events", "item_list")}
	output := fields[0].Output
	parsed, err := parseConfig(config, fields)
	require.NoError(t, err)

	config["region"] = "changed"
	config["tables"].([]map[string]any)[0]["key_schema"].([]map[string]any)[0]["attribute_name"] = "changed"
	fields[0].Config["result"].(map[string]any)["type"] = "item_map"
	fields[0].Field = "changed"

	require.Equal(t, &providerConfig{
		Endpoint: "http://localhost:4566", Region: "us-east-1", AccessKeyID: "test", SecretAccessKey: "test",
		Tables: []dynamofield.Table{{
			Name:                 "events",
			AttributeDefinitions: []dynamofield.AttributeDefinition{{AttributeName: "id", AttributeType: dynamofield.AttributeTypeString}},
			KeySchema:            []dynamofield.KeySchemaElement{{AttributeName: "id", KeyType: dynamofield.KeyTypeHash}},
		}},
		Fields: []dynamofield.Request{{Field: "DynamoDB.events", Config: dynamofield.Config{Table: "events", Result: dynamofield.ResultConfig{Type: dynamofield.ResultTypeItemList}}, Output: output}},
	}, parsed)
}
