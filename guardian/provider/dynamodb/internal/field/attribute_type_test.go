package field

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttributeType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  AttributeType
	}{
		{input: `"S"`, want: AttributeTypeString},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got AttributeType
			require.NoError(t, json.Unmarshal([]byte(tt.input), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAttributeType_UnmarshalJSONRejectsUnsupportedValues(t *testing.T) {
	tests := []string{`"N"`, `""`, `null`, `123`, `true`, `[]`, `{}`}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := AttributeTypeString
			err := json.Unmarshal([]byte(input), &got)
			require.ErrorContains(t, err, "DynamoDB attribute_type")
			require.Equal(t, AttributeTypeString, got)
		})
	}
}
