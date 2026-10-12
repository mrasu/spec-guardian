//go:build specguardian

package specguardian_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"spec-guardian/examples/iceberg/conformance/specguardian/file"

	iceberggo "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
)

// modelFiles maps model references to files prepared for one attempt.
type modelFiles struct {
	environment       *IcebergTableEnvironment
	input             FizzbeeIcebergTableState
	dataFiles         []iceberggo.DataFile
	manifests         map[string]iceberggo.ManifestFile
	manifestLists     map[string]string
	metadataLocations map[string]string
}

func NewModelFiles(t *testing.T, environment *IcebergTableEnvironment, input FizzbeeIcebergTableState) *modelFiles {
	t.Helper()
	require.NotNil(t, environment)
	require.NotNil(t, environment.table)
	return &modelFiles{
		environment: environment, input: input,
		dataFiles:         make([]iceberggo.DataFile, len(input.Fields.ObjectStore.DataFiles)),
		manifests:         make(map[string]iceberggo.ManifestFile),
		manifestLists:     make(map[string]string),
		metadataLocations: make(map[string]string),
	}
}

func (e *IcebergTableEnvironment) seedModelFiles(t *testing.T, input FizzbeeIcebergTableState) {
	t.Helper()
	files := NewModelFiles(t, e, input)
	// Resolve dependencies before writing files that reference them.
	files.writeDataFiles(t)
	files.writeDeleteFiles(t)
	files.writeManifestFiles(t)
	files.writeManifestLists(t)
	files.writeMetadataFiles(t)
	files.registerTable(t)
}

func (s *modelFiles) writeDataFiles(t *testing.T) {
	t.Helper()
	for i, model := range s.input.Fields.ObjectStore.DataFiles {
		rows := make([]file.IcebergFileParquetRow, len(model.Rows))
		for j, row := range model.Rows {
			values := toIcebergRow(t, row)
			rows[j] = file.IcebergFileParquetRow{Col1: values[0], Col2: values[1], Col3: values[2]}
		}
		var contents bytes.Buffer
		require.NoError(t, parquet.Write(&contents, rows))
		location := s.location("data/model-" + model.FilePath + ".parquet")
		s.put(t, location, contents.Bytes())
		s.dataFiles[i] = s.newDataFile(t, location, len(rows), contents.Len())
	}
}

func (s *modelFiles) newDataFile(t *testing.T, location string, rows, size int) iceberggo.DataFile {
	t.Helper()
	builder, err := iceberggo.NewDataFileBuilder(
		s.environment.table.Spec(), iceberggo.EntryContentData, location,
		iceberggo.ParquetFile, nil, nil, nil, int64(rows), int64(size),
	)
	require.NoError(t, err)
	return builder.Build()
}

func (s *modelFiles) writeDeleteFiles(t *testing.T) {
	t.Helper()
	for _, model := range s.input.Fields.ObjectStore.DeleteFiles {
		rows := make([]file.PositionDeleteRow, len(model.File))
		for i, row := range model.File {
			require.GreaterOrEqual(t, row.FileIndex, int64(0), "delete references missing data file %d", row.FileIndex)
			require.Less(t, row.FileIndex, int64(len(s.dataFiles)), "delete references missing data file %d", row.FileIndex)
			data := s.dataFiles[row.FileIndex]
			rows[i].FilePath, rows[i].Pos = data.FilePath(), row.RowIndex
		}
		var contents bytes.Buffer
		require.NoError(t, parquet.Write(&contents, rows))
		location := s.location("data/model-" + model.Location + ".parquet")
		s.put(t, location, contents.Bytes())
	}
}

func (s *modelFiles) writeManifestFiles(t *testing.T) {
	t.Helper()
	for index, model := range s.input.Fields.ObjectStore.ManifestFiles {
		if _, exists := s.manifests[model.Location]; exists {
			continue
		}
		entries := s.toManifestEntries(t, index)
		var contents bytes.Buffer
		location := s.location("metadata/model-" + model.Location + ".avro")
		manifest, err := iceberggo.WriteManifest(location, &contents, s.environment.table.Metadata().Version(),
			s.environment.table.Spec(), s.environment.table.Schema(), model.File.SnapshotId, entries)
		require.NoError(t, err)
		s.put(t, location, contents.Bytes())
		s.manifests[model.Location] = manifest
	}
}

func (s *modelFiles) toManifestEntries(t *testing.T, index int) []iceberggo.ManifestEntry {
	t.Helper()
	model := s.input.Fields.ObjectStore.ManifestFiles[index]
	statuses := map[string]iceberggo.ManifestEntryStatus{"ADDED": iceberggo.EntryStatusADDED, "EXISTING": iceberggo.EntryStatusEXISTING, "DELETED": iceberggo.EntryStatusDELETED}
	entries := make([]iceberggo.ManifestEntry, len(model.File.Entries))
	for i, entry := range model.File.Entries {
		require.GreaterOrEqual(t, entry.DataFile.FileIndex, int64(0), "manifest references missing data file %d", entry.DataFile.FileIndex)
		require.Less(t, entry.DataFile.FileIndex, int64(len(s.dataFiles)), "manifest references missing data file %d", entry.DataFile.FileIndex)
		data := s.dataFiles[entry.DataFile.FileIndex]
		require.Equal(t, "DATA", entry.DataFile.Content, "delete manifest references need a delete file location")
		status, exists := statuses[entry.Status]
		require.True(t, exists, "unknown manifest status %s", entry.Status)
		entries[i] = iceberggo.NewManifestEntry(status, &entry.SnapshotId, &entry.DataSequenceNo, &entry.FileSequenceNo, data)
	}
	return entries
}

func (s *modelFiles) writeManifestLists(t *testing.T) {
	t.Helper()
	for index, model := range s.input.Fields.ObjectStore.ManifestLists {
		if _, exists := s.manifestLists[model.Location]; exists {
			continue
		}
		manifests := s.toManifestFiles(t, index)
		var snapshotID, sequenceNo int64
		for _, entry := range model.Entries {
			snapshotID, sequenceNo = entry.SnapshotId, max(sequenceNo, entry.SequenceNo)
		}
		var contents bytes.Buffer
		require.NoError(t, iceberggo.WriteManifestList(s.environment.table.Metadata().Version(), &contents, snapshotID, nil, &sequenceNo, 0, manifests))
		location := s.location("metadata/snap-model-" + model.Location + ".avro")
		s.put(t, location, contents.Bytes())
		s.manifestLists[model.Location] = location
	}
}

func (s *modelFiles) toManifestFiles(t *testing.T, index int) []iceberggo.ManifestFile {
	t.Helper()
	model := s.input.Fields.ObjectStore.ManifestLists[index]
	manifests := make([]iceberggo.ManifestFile, len(model.Entries))
	for i, entry := range model.Entries {
		manifest, exists := s.manifests[entry.ManifestPath]
		require.True(t, exists, "manifest list references missing manifest %s", entry.ManifestPath)
		manifests[i] = iceberggo.NewManifestFile(s.environment.table.Metadata().Version(), manifest.FilePath(), manifest.Length(), manifest.PartitionSpecID(), entry.SnapshotId).
			Content(manifest.ManifestContent()).SequenceNum(entry.SequenceNo, entry.MinSequenceNo).
			AddedFiles(int32(entry.AddedFileCount)).ExistingFiles(int32(entry.ExistingFileCount)).DeletedFiles(int32(entry.DeletedFileCount)).Build()
	}
	return manifests
}

func (s *modelFiles) writeMetadataFiles(t *testing.T) {
	t.Helper()
	for index, model := range s.input.Fields.ObjectStore.MetadataFiles {
		var metadata map[string]any
		decodeModelValue(t, s.environment.table.Metadata(), &metadata)

		metadata["snapshots"] = s.toSnapshots(t, index)
		metadata["last-sequence-number"] = model.LastSequenceNo
		delete(metadata, "snapshot-log")
		delete(metadata, "refs")
		if model.CurrentSnapshot == nil {
			delete(metadata, "current-snapshot-id")
		} else {
			metadata["current-snapshot-id"] = model.CurrentSnapshot.SnapshotId
			metadata["refs"] = map[string]any{"main": map[string]any{
				"snapshot-id": model.CurrentSnapshot.SnapshotId, "type": "branch",
			}}
		}
		contents, err := json.Marshal(metadata)
		require.NoError(t, err)
		_, err = table.ParseMetadataBytes(contents)
		require.NoError(t, err)
		location := s.location(fmt.Sprintf("metadata/%05d-model-%s.metadata.json", model.Version, model.Location))
		if model.Version == 0 {
			location = s.environment.table.MetadataLocation()
		}
		s.put(t, location, contents)
		s.metadataLocations[model.Location] = location
	}
}

func (s *modelFiles) toSnapshots(t *testing.T, index int) []table.Snapshot {
	t.Helper()
	model := s.input.Fields.ObjectStore.MetadataFiles[index]
	snapshots := make([]table.Snapshot, len(model.Snapshots))
	for i, snapshot := range model.Snapshots {
		location, exists := s.manifestLists[snapshot.ManifestListPath]
		require.True(t, exists, "snapshot references missing manifest list %s", snapshot.ManifestListPath)
		snapshots[i] = table.Snapshot{SnapshotID: snapshot.SnapshotId, ParentSnapshotID: snapshot.ParentSnapshotId,
			SequenceNumber: snapshot.SequenceNo, TimestampMs: s.environment.table.Metadata().LastUpdatedMillis(),
			ManifestList: location, Summary: &table.Summary{Operation: table.Operation(strings.ToLower(snapshot.Operation))}}
	}
	return snapshots
}

func (s *modelFiles) registerTable(t *testing.T) {
	t.Helper()
	location, exists := s.metadataLocations[s.input.Fields.Catalog.MetadataLocation]
	require.True(t, exists, "catalog metadata is missing from model")
	require.NoError(t, s.environment.catalog.DropTable(t.Context(), s.environment.identifier))
	registered, err := s.environment.catalog.RegisterTable(t.Context(), s.environment.identifier, location)
	require.NoError(t, err)
	s.environment.table = registered
}

func (s *modelFiles) location(relative string) string {
	return strings.TrimSuffix(s.environment.table.Location(), "/") + "/" + relative
}

func (s *modelFiles) put(t *testing.T, location string, contents []byte) {
	t.Helper()
	key, found := strings.CutPrefix(location, "s3://"+conformanceBucket+"/")
	require.True(t, found, "unexpected table location %s", location)
	_, err := s.environment.s3.PutObject(t.Context(), &s3.PutObjectInput{
		Bucket: aws.String(conformanceBucket), Key: aws.String(key), Body: bytes.NewReader(contents),
	})
	require.NoError(t, err)
}
