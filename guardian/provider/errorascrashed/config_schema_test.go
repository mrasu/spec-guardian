package errorascrashed

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
		{name: "omitted settings", shared: `null`, field: `null`, valid: true},
		{name: "empty settings", shared: `{}`, field: `{}`, valid: true},
		{name: "unknown shared setting", shared: `typo: true`, field: ``, valid: false},
		{name: "unknown field setting", shared: `{}`, field: `typo: true`, valid: false},
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

func TestParseConfig_KeepsFieldMetadata(t *testing.T) {
	output := provider.ActionCrashedOutputSpec()
	fields := []provider.FieldRequest{{Field: provider.ActionCrashedField, Output: output}}
	parsed, err := parseConfig(nil, fields)
	require.NoError(t, err)

	fields[0].Field = "changed"
	require.Equal(t, &providerConfig{Fields: []fieldRequest{{Field: provider.ActionCrashedField, Output: output}}}, parsed)
}
