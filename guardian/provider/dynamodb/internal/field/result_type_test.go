package field

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResultType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  ResultType
	}{
		{input: `"item_map"`, want: ResultTypeItemMap},
		{input: `"item_list"`, want: ResultTypeItemList},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got ResultType
			require.NoError(t, json.Unmarshal([]byte(tt.input), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResultType_UnmarshalJSONRejectsUnsupportedValues(t *testing.T) {
	tests := []string{`"kv"`, `""`, `null`, `123`, `true`, `[]`, `{}`}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := ResultTypeItemMap
			err := json.Unmarshal([]byte(input), &got)
			require.ErrorContains(t, err, "DynamoDB result.type")
			require.Equal(t, ResultTypeItemMap, got)
		})
	}
}
