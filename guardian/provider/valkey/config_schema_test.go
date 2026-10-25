package valkey

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
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
		{name: "key pattern", shared: `address: localhost:16379`, field: `keys: "*"`, valid: true},
		{name: "keyspace", shared: `address: localhost:16379
keyspaces: [{name: records, prefix: "", data_type: string}]`, field: `keyspace: records`, valid: true},
		{name: "default prefix", shared: `address: localhost:16379
keyspaces: [{name: records, data_type: string}]`, field: `keyspace: records`, valid: true},
		{name: "missing address", shared: `{}`, field: ``, valid: false},
		{name: "unknown shared setting", shared: `address: localhost:16379
typo: true`, field: ``, valid: false},
		{name: "unknown keyspace setting", shared: `address: localhost:16379
keyspaces: [{name: records, data_type: string, typo: true}]`, field: ``, valid: false},
		{name: "invalid prefix", shared: `address: localhost:16379
keyspaces: [{name: records, prefix: 123, data_type: string}]`, field: ``, valid: false},
		{name: "ambiguous scope", shared: `address: localhost:16379`, field: `keyspace: records
keys: "*"`, valid: false},
		{name: "unknown field setting", shared: `address: localhost:16379`, field: `keys: "*"
typo: true`, valid: false},
		{name: "missing scope", shared: `address: localhost:16379`, field: `{}`, valid: false},
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
	instance, err := NewProvider(Config{}, map[string]any{"address": "localhost:16379"}, []provider.FieldRequest{{
		Field:  "role.records",
		Config: map[string]any{"typo": true},
	}})
	require.ErrorContains(t, err, "field role.records config")
	require.ErrorContains(t, err, "additional properties")
	require.Nil(t, instance)
}

func TestParseConfig_DecodesIndependentSettings(t *testing.T) {
	space := map[string]any{"name": "records", "prefix": "test:", "data_type": "string"}
	config := map[string]any{"address": "localhost:16379", "keyspaces": []map[string]any{space}}
	output := provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString}}}
	fields := []provider.FieldRequest{
		{Field: "Cache.records", Config: map[string]any{"keyspace": "records"}, Output: output},
		{Field: "Cache.observed", Config: map[string]any{"keys": "other:*"}, Output: output},
	}
	parsed, err := parseConfig(config, fields)
	require.NoError(t, err)

	config["address"] = "changed:1"
	space["prefix"] = "changed:"
	fields[0].Config["keyspace"] = "changed"
	fields[1].Field = "changed"

	require.Equal(t, &providerConfig{
		Address:   "localhost:16379",
		Keyspaces: []keyspace{{Name: "records", Prefix: "test:", DataType: dataTypeString}},
		Fields: []fieldRequest{
			{Field: "Cache.records", Config: fieldConfig{Keyspace: "records"}, Output: output},
			{Field: "Cache.observed", Config: fieldConfig{Keys: "other:*"}, Output: output},
		},
	}, parsed)
}
