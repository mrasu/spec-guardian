package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSeedFields_PreservesFieldCountAndOrder(t *testing.T) {
	request := SeedRequest{Values: []SeedValue{{Field: "DB.rows"}, {Field: "DB.query"}}}
	tests := []struct {
		name   string
		fields []SeedValue
		want   string
	}{
		{"same fields", []SeedValue{{Field: "DB.rows"}, {Field: "DB.query"}}, ""},
		{"missing field", []SeedValue{{Field: "DB.rows"}}, "returned 1 fields"},
		{"extra field", []SeedValue{{Field: "DB.rows"}, {Field: "DB.query"}, {Field: "extra"}}, "returned 3 fields"},
		{"reordered fields", []SeedValue{{Field: "DB.query"}, {Field: "DB.rows"}}, "seed field 0"},
		{"renamed field", []SeedValue{{Field: "other"}, {Field: "DB.query"}}, "seed field 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSeedFields(request, SeedResult{Values: tt.fields})
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}

func TestValidateSeedValue_PreservesStructureValuesAndListPositions(t *testing.T) {
	type row struct {
		ID     int64               `json:"id"`
		Values map[string][]string `json:"values"`
	}
	expected := []row{{ID: 1, Values: map[string][]string{"key": {"a", "b"}}}, {ID: 2, Values: map[string][]string{}}}
	tests := []struct {
		name  string
		value any
		valid bool
	}{
		{"typed values", expected, true},
		{"provider messages", []map[string]any{{"id": int64(1), "values": map[string][]string{"key": {"a", "b"}}}, {"id": int64(2), "values": map[string][]string{}}}, true},
		{"reordered rows", []row{expected[1], expected[0]}, false},
		{"missing row", expected[:1], false},
		{"extra row", append(append([]row{}, expected...), expected[0]), false},
		{"reordered nested list", []row{{ID: 1, Values: map[string][]string{"key": {"b", "a"}}}, expected[1]}, false},
		{"changed key", []row{{ID: 1, Values: map[string][]string{"other": {"a", "b"}}}, expected[1]}, false},
		{"changed value", []row{{ID: 3, Values: expected[0].Values}, expected[1]}, false},
		{"missing message field", []map[string]any{{"id": int64(1)}, {"id": int64(2), "values": map[string][]string{}}}, false},
		{"extra message field", []map[string]any{{"id": int64(1), "values": expected[0].Values, "extra": true}, {"id": int64(2), "values": map[string][]string{}}}, false},
		{"null list", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSeedValue("DB.rows", expected, tt.value)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateSeedValue_ReportsNestedMismatchPaths(t *testing.T) {
	type row struct {
		Values map[string][]string `json:"values"`
	}
	expected := []row{{Values: map[string][]string{"key": {"a", "b"}}}}
	tests := []struct {
		name   string
		actual []row
		path   string
	}{
		{"changed value", []row{{Values: map[string][]string{"key": {"a", "changed"}}}}, `DB.rows[0].values["key"][1]`},
		{"missing key", []row{{Values: map[string][]string{}}}, `DB.rows[0].values["key"]`},
		{"extra key", []row{{Values: map[string][]string{"key": {"a", "b"}, "extra": {}}}}, `DB.rows[0].values["extra"]`},
		{"list length", []row{{Values: map[string][]string{"key": {"a"}}}}, `DB.rows[0].values["key"]: seed list length`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSeedValue("DB.rows", expected, tt.actual)
			require.ErrorContains(t, err, tt.path)
		})
	}
}
