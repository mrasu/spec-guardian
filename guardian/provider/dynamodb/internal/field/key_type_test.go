package field

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  KeyType
	}{
		{input: `"HASH"`, want: KeyTypeHash},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got KeyType
			require.NoError(t, json.Unmarshal([]byte(tt.input), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestKeyType_UnmarshalJSONRejectsUnsupportedValues(t *testing.T) {
	tests := []string{`"RANGE"`, `""`, `null`, `123`, `true`, `[]`, `{}`}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := KeyTypeHash
			err := json.Unmarshal([]byte(input), &got)
			require.ErrorContains(t, err, "DynamoDB key_type")
			require.Equal(t, KeyTypeHash, got)
		})
	}
}
