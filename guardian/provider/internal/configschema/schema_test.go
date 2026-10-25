package configschema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSchemas_RejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{name: "invalid YAML", document: "[", want: "read config schema"},
		{name: "missing provider definition", document: "$defs:\n  fieldConfig:\n    type: object\n", want: "compile provider config schema"},
		{name: "missing field definition", document: "$defs:\n  providerConfig:\n    type: object\n", want: "compile field config schema"},
		{name: "invalid schema", document: "$defs:\n  providerConfig:\n    type: unknown\n  fieldConfig:\n    type: object\n", want: "compile provider config schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemas, err := NewSchemas([]byte(tt.document), "urn:test:invalid")
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, schemas)
		})
	}
}

func TestSchemas_DecodeProvider_AcceptsOmittedSettings(t *testing.T) {
	schemas, err := NewSchemas([]byte(`
$defs:
  providerConfig:
    type: object
    additionalProperties: false
  fieldConfig:
    type: object
    required: [name]
    properties:
      name:
        type: string
`), "urn:test:omitted")
	require.NoError(t, err)

	var shared struct{}
	require.NoError(t, schemas.DecodeProvider(nil, &shared))
	var field struct{ Name string }
	require.ErrorContains(t, schemas.DecodeField(nil, &field), "missing property 'name'")
	require.Equal(t, "", field.Name)
}

func TestSchemas_DecodeField_PreservesIntegerPrecision(t *testing.T) {
	schemas, err := NewSchemas([]byte(`
$defs:
  providerConfig:
    type: object
  fieldConfig:
    type: object
    required: [value]
    properties:
      value:
        type: integer
        const: 9007199254740993
`), "urn:test:integer")
	require.NoError(t, err)

	var result struct{ Value int64 }
	err = schemas.DecodeField(map[string]any{"value": int64(9007199254740993)}, &result)
	require.NoError(t, err)
	require.Equal(t, int64(9007199254740993), result.Value)

	err = schemas.DecodeField(map[string]any{"value": int64(9007199254740992)}, &result)
	require.ErrorContains(t, err, "/value")
	require.Equal(t, int64(9007199254740993), result.Value)
}
