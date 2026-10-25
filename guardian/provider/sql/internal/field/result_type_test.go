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
		{input: `"kv"`, want: ResultTypeKV},
		{input: `"row_map"`, want: ResultTypeRowMap},
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
	tests := []string{`"rows"`, `""`, `null`, `123`, `true`, `[]`, `{}`}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := ResultTypeKV
			err := json.Unmarshal([]byte(input), &got)
			require.ErrorContains(t, err, "SQL result.type")
			require.Equal(t, ResultTypeKV, got)
		})
	}
}
