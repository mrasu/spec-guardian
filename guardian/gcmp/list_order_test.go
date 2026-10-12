package gcmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEqualIgnoringListOrder_PreservesContentsAndDuplicates(t *testing.T) {
	type manifest struct {
		Location string `json:"location"`
		Status   string `json:"status"`
	}
	want := []manifest{{"model-1", "ADDED"}, {"model-2", "ADDED"}, {"model-3", "DELETED"}}
	comparison := EqualIgnoringListOrder(want, []string{""}, "[]/location")
	tests := []struct {
		name    string
		got     []manifest
		matches bool
	}{
		{"reordered files", []manifest{{"s3-3", "DELETED"}, {"s3-2", "ADDED"}, {"s3-1", "ADDED"}}, true},
		{"different content", []manifest{{"s3-1", "EXISTING"}, {"s3-2", "ADDED"}, {"s3-3", "DELETED"}}, false},
		{"missing duplicate", []manifest{{"s3-1", "ADDED"}, {"s3-3", "DELETED"}}, false},
		{"different multiplicity", []manifest{{"s3-1", "ADDED"}, {"s3-2", "DELETED"}, {"s3-3", "DELETED"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Observed(tt.got).MatchComparison("manifest_files", comparison)
			if tt.matches {
				require.NoError(t, err)
				require.NoError(t, comparison.Match("manifest_files", tt.got))
			} else {
				require.ErrorContains(t, err, "manifest_files")
				require.ErrorContains(t, comparison.Match("manifest_files", tt.got), "manifest_files")
			}
		})
	}
	require.Equal(t, []manifest{{"model-1", "ADDED"}, {"model-2", "ADDED"}, {"model-3", "DELETED"}}, want)
}

func TestEqualIgnoringListOrder_NormalizesNestedListsBeforeOuterList(t *testing.T) {
	want := [][][]string{{{"a", "b"}, {"c", "d"}}, {{"a", "c"}}}
	got := [][][]string{{{"a", "c"}}, {{"c", "d"}, {"a", "b"}}}
	require.NoError(t, Observed(got).MatchComparison("files", EqualIgnoringListOrder(want, []string{"", "[]"})))
	require.Error(t, Observed(got).MatchComparison("files", EqualIgnoringListOrder(want, []string{""})))
	changedColumnOrder := [][][]string{{{"a", "c"}}, {{"d", "c"}, {"a", "b"}}}
	require.Error(t, Observed(changedColumnOrder).MatchComparison("files", EqualIgnoringListOrder(want, []string{"", "[]"})))
	require.Equal(t, [][][]string{{{"a", "c"}}, {{"c", "d"}, {"a", "b"}}}, got)
}

func TestEqualIgnoringListOrder_SupportsListsInMapValues(t *testing.T) {
	type record struct {
		Rows []string `json:"rows"`
	}
	want := map[string]*record{"live": {Rows: []string{"a", "b"}}, "empty": nil}
	got := map[string]*record{"live": {Rows: []string{"b", "a"}}, "empty": nil}
	require.NoError(t, Observed(got).MatchComparison("records", EqualIgnoringListOrder(want, []string{"{}/rows"})))
}

func TestEqualIgnoringListOrder_RejectsNonList(t *testing.T) {
	require.ErrorContains(t, Observed("value").MatchComparison("field", EqualIgnoringListOrder("value", []string{""})), "expected a list")
}
