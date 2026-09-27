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
	require.Len(t, o.row, 3)
	return icebergRow{o.row[0], o.row[1], o.row[2]}
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
	objects map[string]json.RawMessage
}

func NewModelObjectStore(t *testing.T, objects map[string]any) *modelObjectStore {
	t.Helper()
	require.NotNil(t, objects)
	var decoded map[string]json.RawMessage
	decodeModelValue(t, objects, &decoded)
	return &modelObjectStore{objects: decoded}
}

// scanLiveFiles follows only the catalog's committed snapshot.
func (s *modelObjectStore) scanLiveFiles(t *testing.T, metadataLocation string) [][]icebergRow {
	t.Helper()
	metadata := s.findMetadata(t, metadataLocation)
	if metadata.CurrentSnapshot == nil {
		return nil
	}
	return s.scanSnapshot(t, *metadata.CurrentSnapshot)
}

func (s *modelObjectStore) scanSnapshot(t *testing.T, snapshot modelSnapshot) [][]icebergRow {
	t.Helper()
	manifestList := s.findManifestList(t, snapshot.ManifestListPath)
	var files [][]icebergRow
	for _, listed := range manifestList {
		manifest := s.findManifest(t, listed.ManifestPath)
		files = append(files, s.scanManifest(t, manifest)...)
	}
	return files
}

func (s *modelObjectStore) scanManifest(t *testing.T, manifest modelManifest) [][]icebergRow {
	t.Helper()
	var files [][]icebergRow
	for _, entry := range manifest.Entries {
		if entry.Status == "DELETED" || entry.DataFile.Content != "DATA" {
			continue
		}
		dataFile := s.findDataFile(t, entry.DataFile.FilePath)
		files = append(files, scanDataFileRows(t, dataFile))
	}
	return files
}

func (s *modelObjectStore) findMetadata(t *testing.T, location string) modelMetadata {
	t.Helper()
	var metadata modelMetadata
	s.decodeFile(t, location, &metadata)
	return metadata
}

func (s *modelObjectStore) findManifestList(t *testing.T, path string) []modelManifestListEntry {
	t.Helper()
	var manifestList []modelManifestListEntry
	s.decodeFile(t, path, &manifestList)
	return manifestList
}

func (s *modelObjectStore) findManifest(t *testing.T, path string) modelManifest {
	t.Helper()
	var manifest modelManifest
	s.decodeFile(t, path, &manifest)
	return manifest
}

func (s *modelObjectStore) findDataFile(t *testing.T, path string) modelDataFile {
	t.Helper()
	var dataFile modelDataFile
	s.decodeFile(t, path, &dataFile)
	return dataFile
}

func (s *modelObjectStore) decodeFile(t *testing.T, path string, destination any) {
	t.Helper()
	contents, found := s.objects[path]
	require.True(t, found, "model object %q is missing", path)
	require.NoError(t, json.Unmarshal(contents, destination), "decode model object %q", path)
}

type modelMetadata struct {
	CurrentSnapshot *modelSnapshot `json:"current_snapshot"`
}

type modelSnapshot struct {
	ManifestListPath string `json:"manifest_list_path"`
}

type modelManifestListEntry struct {
	ManifestPath string `json:"manifest_path"`
}

type modelManifest struct {
	Entries []modelManifestEntry `json:"entries"`
}

type modelManifestEntry struct {
	Status   string `json:"status"`
	DataFile struct {
		Content  string `json:"content"`
		FilePath string `json:"file_path"`
	} `json:"data_file"`
}

type modelDataFile struct {
	Rows [][]string `json:"rows"`
}

func scanDataFileRows(t *testing.T, file modelDataFile) []icebergRow {
	t.Helper()
	rows := make([]icebergRow, 0, len(file.Rows))
	for _, values := range file.Rows {
		require.Len(t, values, 3)
		rows = append(rows, icebergRow{values[0], values[1], values[2]})
	}
	return rows
}

func decodeModelValue(t *testing.T, value, destination any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, destination))
}

func modelLiveFiles(t *testing.T, state FizzbeeIcebergTableState) [][]icebergRow {
	t.Helper()
	store := NewModelObjectStore(t, state.Fields.ObjectStore.Objects)
	return store.scanLiveFiles(t, state.Fields.Catalog.MetadataLocation)
}

func modelDataFiles(t *testing.T, state FizzbeeIcebergTableState) []dataFileRows {
	t.Helper()
	modelFiles := modelLiveFiles(t, state)
	files := make([]dataFileRows, 0, len(modelFiles))
	for _, rows := range modelFiles {
		files = append(files, dataFileRows{Rows: append([]icebergRow{}, rows...)})
	}
	sortDataFiles(files)
	return files
}
