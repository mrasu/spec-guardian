//go:build specguardian

package specguardian_test

import (
	"slices"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go/table"
	"github.com/stretchr/testify/require"
)

// dataFileRows represents one live data file, independent of its S3 key.
type dataFileRows struct {
	Rows []icebergRow
}

func scanDataFiles(t *testing.T, current *table.Table) []dataFileRows {
	t.Helper()
	scan := current.Scan()
	tasks, err := scan.PlanFiles(t.Context())
	require.NoError(t, err)
	files := make([]dataFileRows, 0, len(tasks))
	for _, task := range tasks {
		files = append(files, dataFileRows{Rows: scanTaskRows(t, scan, task)})
	}
	sortDataFiles(files)
	return files
}

func scanTaskRows(t *testing.T, scan *table.Scan, task table.FileScanTask) []icebergRow {
	t.Helper()
	// Compare the rows stored in the data file, before applying delete files.
	task.DeleteFiles = nil
	task.EqualityDeleteFiles = nil
	task.DeletionVectorFiles = nil
	_, batches, err := scan.ReadTasks(t.Context(), []table.FileScanTask{task})
	require.NoError(t, err)
	rows := []icebergRow{}
	for batch, err := range batches {
		require.NoError(t, err)
		for index := 0; index < int(batch.NumRows()); index++ {
			row := icebergRow{}
			for column := range row {
				row[column] = batch.Column(column).(*array.String).Value(index)
			}
			rows = append(rows, row)
		}
		batch.Release()
	}
	return rows
}

func sortDataFiles(files []dataFileRows) {
	compareRows := func(a, b icebergRow) int {
		return slices.Compare(a[:], b[:])
	}
	for index := range files {
		slices.SortFunc(files[index].Rows, compareRows)
	}
	slices.SortFunc(files, func(a, b dataFileRows) int {
		return slices.CompareFunc(a.Rows, b.Rows, compareRows)
	})
}
