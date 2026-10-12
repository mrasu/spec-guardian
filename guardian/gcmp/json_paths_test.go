package gcmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEqualIgnoringJSONPathsKeepsOtherFields(t *testing.T) {
	type dataFile struct {
		FilePath string     `json:"file_path"`
		Rows     [][]string `json:"rows"`
	}
	observed := Observed([]dataFile{{FilePath: "s3://bucket/random.parquet", Rows: [][]string{{"jack", "apple"}}}})
	allowed := EqualIgnoringJSONPaths([]dataFile{{FilePath: "data-1", Rows: [][]string{{"jack", "apple"}}}}, "[]/file_path")
	require.NoError(t, observed.MatchComparison("data_files", allowed))

	changedRows := EqualIgnoringJSONPaths([]dataFile{{FilePath: "data-1", Rows: [][]string{{"jack", "orange"}}}}, "[]/file_path")
	require.ErrorContains(t, observed.MatchComparison("data_files", changedRows), "data_files")

	missingFile := EqualIgnoringJSONPaths([]dataFile{}, "[]/file_path")
	require.ErrorContains(t, observed.MatchComparison("data_files", missingFile), "data_files")
}
