package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneSeedRequest_IsolatesNestedModelValues(t *testing.T) {
	type row struct {
		Name    *string             `json:"name"`
		Payload []byte              `json:"payload"`
		Values  map[string][]string `json:"values"`
	}
	name := "original"
	rows := []row{{Name: &name, Payload: []byte{1, 2}, Values: map[string][]string{"key": {"original"}}}}
	request := SeedRequest{Values: []SeedValue{
		{Field: "DB.rows", Value: rows},
		{Field: "DB.dynamic", Value: map[string]any{"nested": []any{map[string]any{"value": "original"}}}},
	}}
	cloned := CloneSeedRequest(request)
	require.Equal(t, request, cloned)
	copiedRows := cloned.Values[0].Value.([]row)
	*copiedRows[0].Name = "changed"
	copiedRows[0].Payload[0] = 9
	copiedRows[0].Values["key"][0] = "changed"
	cloned.Values[0].Field = "renamed"
	cloned.Values[1].Value.(map[string]any)["nested"].([]any)[0].(map[string]any)["value"] = "changed"
	require.Equal(t, "original", name)
	require.Equal(t, []row{{Name: &name, Payload: []byte{1, 2}, Values: map[string][]string{"key": {"original"}}}}, rows)
	require.Equal(t, SeedRequest{Values: []SeedValue{
		{Field: "DB.rows", Value: rows},
		{Field: "DB.dynamic", Value: map[string]any{"nested": []any{map[string]any{"value": "original"}}}},
	}}, request)
}

func TestCloneSeedRequest_PreservesNullAndEmptyValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"null", nil}, {"nil list", []string(nil)}, {"empty list", []string{}},
		{"nil map", map[string]string(nil)}, {"empty map", map[string]string{}},
		{"nil pointer", (*string)(nil)}, {"empty bytes", []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := SeedRequest{Values: []SeedValue{{Field: "field", Value: tt.value}}}
			require.Equal(t, request, CloneSeedRequest(request))
		})
	}
	require.Equal(t, SeedRequest{}, CloneSeedRequest(SeedRequest{}))
	require.Equal(t, SeedRequest{Values: []SeedValue{}}, CloneSeedRequest(SeedRequest{Values: []SeedValue{}}))
}
