package valkey

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  dataType
	}{
		{input: `"string"`, want: dataTypeString},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got dataType
			require.NoError(t, json.Unmarshal([]byte(tt.input), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDataType_UnmarshalJSONRejectsUnsupportedValues(t *testing.T) {
	tests := []string{`"list"`, `""`, `null`, `123`, `true`, `[]`, `{}`}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := dataTypeString
			err := json.Unmarshal([]byte(input), &got)
			require.ErrorContains(t, err, "Valkey data_type")
			require.Equal(t, dataTypeString, got)
		})
	}
}
