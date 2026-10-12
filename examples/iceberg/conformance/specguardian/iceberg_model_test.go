//go:build specguardian

package specguardian_test

import (
	"encoding/json"
	"testing"

	iceberggo "github.com/apache/iceberg-go"
	"github.com/stretchr/testify/require"
)

type icebergRow [3]string

// modelOperation converts generated action parameters into typed columns.
type modelOperation struct {
	row       []string
	predicate []*string
	setValues []*string
}

func NewModelOperation(t *testing.T, state FizzbeeIcebergTableState) *modelOperation {
	t.Helper()
	var params struct {
		Row       []string  `json:"row"`
		Predicate []*string `json:"predicate"`
		SetValues []*string `json:"set_values"`
	}
	decodeModelValue(t, state.Params.IcebergTable, &params)
	require.True(t, len(params.Row) == 0 || len(params.Row) == 3, "row must have 3 columns")
	require.True(t, len(params.Predicate) == 0 || len(params.Predicate) == 3, "predicate must have 3 columns")
	require.True(t, len(params.SetValues) == 0 || len(params.SetValues) == 3, "set_values must have 3 columns")
	return &modelOperation{
		row:       params.Row,
		predicate: params.Predicate,
		setValues: params.SetValues,
	}
}

func (o *modelOperation) appendRow(t *testing.T) icebergRow {
	t.Helper()
	return toIcebergRow(t, o.row)
}

func (o *modelOperation) predicateExpression(t *testing.T) iceberggo.BooleanExpression {
	t.Helper()
	require.Len(t, o.predicate, 3)
	names := [...]string{"col1", "col2", "col3"}
	var expression iceberggo.BooleanExpression
	for column, value := range o.predicate {
		if value == nil {
			continue
		}
		match := iceberggo.EqualTo(iceberggo.Reference(names[column]), *value)
		if expression == nil {
			expression = match
			continue
		}
		expression = iceberggo.NewAnd(expression, match)
	}
	require.NotNil(t, expression)
	return expression
}

func (o *modelOperation) updatedRows(t *testing.T, state FizzbeeIcebergTableState) []icebergRow {
	t.Helper()
	require.Len(t, o.predicate, 3)
	require.Len(t, o.setValues, 3)
	var updated []icebergRow
	for _, fileRows := range modelLiveFiles(t, state) {
		for _, row := range fileRows {
			if !o.matches(row) {
				continue
			}
			updated = append(updated, o.replaceValues(row))
		}
	}
	return updated
}

func (o *modelOperation) matches(row icebergRow) bool {
	for column, value := range o.predicate {
		if value != nil && row[column] != *value {
			return false
		}
	}
	return true
}

func (o *modelOperation) replaceValues(row icebergRow) icebergRow {
	for column, value := range o.setValues {
		if value != nil {
			row[column] = *value
		}
	}
	return row
}

// modelObjectStore resolves the files in one FizzBee object store snapshot.
type modelObjectStore struct {
	metadataFiles           ObjectStoreMetadataFilesValue
	manifestLists           ObjectStoreManifestListsValue
	manifestFiles           ObjectStoreManifestFilesValue
	dataFiles               ObjectStoreDataFilesValue
	metadataByLocation      map[string]int
	manifestListsByLocation map[string]int
	manifestsByLocation     map[string]int
}

func NewModelObjectStore(t *testing.T, metadataFiles ObjectStoreMetadataFilesValue, manifestLists ObjectStoreManifestListsValue, manifestFiles ObjectStoreManifestFilesValue, dataFiles ObjectStoreDataFilesValue) *modelObjectStore {
	t.Helper()
	s := &modelObjectStore{
		metadataFiles: metadataFiles, manifestLists: manifestLists, manifestFiles: manifestFiles, dataFiles: dataFiles,
		metadataByLocation: make(map[string]int), manifestListsByLocation: make(map[string]int), manifestsByLocation: make(map[string]int),
	}
	for i, value := range metadataFiles {
		require.NotEmpty(t, value.Location)
		s.metadataByLocation[value.Location] = i
	}
	for i, value := range manifestLists {
		require.NotEmpty(t, value.Location)
		s.manifestListsByLocation[value.Location] = i
	}
	for i, value := range manifestFiles {
		require.NotEmpty(t, value.Location)
		s.manifestsByLocation[value.Location] = i
	}
	return s
}

// scanLiveFiles follows only the catalog's committed snapshot.
func (s *modelObjectStore) scanLiveFiles(t *testing.T, metadataLocation string) [][]icebergRow {
	t.Helper()
	index, found := s.metadataByLocation[metadataLocation]
	require.True(t, found, "model metadata %q is missing", metadataLocation)
	snapshot := s.metadataFiles[index].CurrentSnapshot
	if snapshot == nil {
		return nil
	}
	return s.scanManifestList(t, snapshot.ManifestListPath)
}

func (s *modelObjectStore) scanManifestList(t *testing.T, location string) [][]icebergRow {
	t.Helper()
	index, found := s.manifestListsByLocation[location]
	require.True(t, found, "model manifest list %q is missing", location)
	var files [][]icebergRow
	for _, entry := range s.manifestLists[index].Entries {
		files = append(files, s.scanManifest(t, entry.ManifestPath)...)
	}
	return files
}

func (s *modelObjectStore) scanManifest(t *testing.T, location string) [][]icebergRow {
	t.Helper()
	index, found := s.manifestsByLocation[location]
	require.True(t, found, "model manifest %q is missing", location)
	var files [][]icebergRow
	for _, entry := range s.manifestFiles[index].File.Entries {
		if entry.Status == "DELETED" || entry.DataFile.Content != "DATA" {
			continue
		}
		fileIndex := entry.DataFile.FileIndex
		require.GreaterOrEqual(t, fileIndex, int64(0))
		require.Less(t, fileIndex, int64(len(s.dataFiles)))
		files = append(files, toIcebergRows(t, s.dataFiles[fileIndex].Rows))
	}
	return files
}

func toIcebergRows(t *testing.T, values [][]string) []icebergRow {
	t.Helper()
	rows := make([]icebergRow, len(values))
	for i, value := range values {
		rows[i] = toIcebergRow(t, value)
	}
	return rows
}

func toIcebergRow(t *testing.T, values []string) icebergRow {
	t.Helper()
	require.Len(t, values, 3)
	return icebergRow{values[0], values[1], values[2]}
}

func decodeModelValue(t *testing.T, value, destination any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, destination))
}

func modelLiveFiles(t *testing.T, state FizzbeeIcebergTableState) [][]icebergRow {
	t.Helper()
	store := NewModelObjectStore(t,
		state.Fields.ObjectStore.MetadataFiles,
		state.Fields.ObjectStore.ManifestLists,
		state.Fields.ObjectStore.ManifestFiles,
		state.Fields.ObjectStore.DataFiles)
	return store.scanLiveFiles(t, state.Fields.Catalog.MetadataLocation)
}
