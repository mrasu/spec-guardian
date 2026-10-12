// Package metadatafiles observes Iceberg table metadata files in S3.
package metadatafiles

import (
	"context"
	"path"
	"reflect"
	"strconv"
	"strings"

	"spec-guardian/examples/iceberg/conformance/specguardian/observer/common"

	"github.com/apache/iceberg-go/table"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/observer"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Observer reads all metadata files under one table prefix.
type Observer struct {
	base *common.BaseObserver
}

type metadataFile struct {
	location        string
	version         int64
	currentSnapshot *table.Snapshot
	snapshots       []table.Snapshot
	lastSequenceNo  int64
}

var _ observer.Observer = (*Observer)(nil)

var (
	snapshotType = observer.Type{Kind: observer.KindMessage, Fields: map[string]observer.Type{
		"sequence_no":        {Kind: observer.KindInt64},
		"snapshot_id":        {Kind: observer.KindInt64},
		"parent_snapshot_id": {Kind: observer.KindInt64, Nullable: true},
		"operation":          {Kind: observer.KindString},
		"manifest_list_path": {Kind: observer.KindString},
	}}
	metadataFilesType = observer.Type{Kind: observer.KindList, Values: &observer.Type{
		Kind: observer.KindMessage,
		Fields: map[string]observer.Type{
			"location":         {Kind: observer.KindString},
			"version":          {Kind: observer.KindInt64},
			"current_snapshot": {Kind: observer.KindMessage, Nullable: true, Fields: snapshotType.Fields},
			"snapshots":        {Kind: observer.KindList, Values: &snapshotType},
			"last_sequence_no": {Kind: observer.KindInt64},
		},
	}}
)

// New creates the metadata file observer used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the field requests and creates a metadata file observer.
func NewObserver(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	base, err := common.NewBaseObserver("metadata file", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Observer{base: base}, nil
}

func validateType(spec observer.Type) error {
	if !reflect.DeepEqual(spec, metadataFilesType) {
		return errors.New("metadata file field requires location, version, current_snapshot, snapshots, and last_sequence_no")
	}
	return nil
}

// SetTable supplies the storage location for the next attempt.
func (o *Observer) SetTable(client *s3.Client, bucket, prefix string) error {
	return o.base.SetTable(client, bucket, prefix)
}

// PrepareCase begins a conformance case.
func (o *Observer) PrepareCase(ctx context.Context) error {
	return o.base.PrepareCase(ctx)
}

// PrepareAttempt checks that setup supplied a table.
func (o *Observer) PrepareAttempt(ctx context.Context) error {
	return o.base.PrepareAttempt(ctx)
}

// CleanupAttempt clears the table assigned to the attempt.
func (o *Observer) CleanupAttempt(ctx context.Context) error {
	return o.base.CleanupAttempt(ctx)
}

// CleanupCase ends a conformance case.
func (o *Observer) CleanupCase(ctx context.Context) error {
	return o.base.CleanupCase(ctx)
}

// Observe returns every metadata file in the table prefix.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	files, err := o.readMetadataFiles(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedMetadataFiles(files), nil
}

func (o *Observer) readMetadataFiles(ctx context.Context) ([]metadataFile, error) {
	keys, err := o.base.TableObjects().ListKeys(ctx, ".metadata.json")
	if err != nil {
		return nil, err
	}
	files := make([]metadataFile, 0, len(keys))
	for _, key := range keys {
		file, err := o.readMetadataFile(ctx, key)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, nil
}

func (o *Observer) readMetadataFile(ctx context.Context, key string) (metadataFile, error) {
	version, err := metadataVersion(key)
	if err != nil {
		return metadataFile{}, err
	}
	contents, err := o.base.TableObjects().Read(ctx, key)
	if err != nil {
		return metadataFile{}, err
	}
	metadata, err := table.ParseMetadataBytes(contents)
	if err != nil {
		return metadataFile{}, errors.Wrapf(err, "parse metadata file %s", key)
	}
	for _, snapshot := range metadata.Snapshots() {
		if snapshot.Summary == nil || snapshot.Summary.Operation == "" {
			return metadataFile{}, errors.Newf("snapshot %d has no operation", snapshot.SnapshotID)
		}
	}
	return metadataFile{
		location:        key,
		version:         version,
		currentSnapshot: metadata.CurrentSnapshot(),
		snapshots:       metadata.Snapshots(),
		lastSequenceNo:  metadata.LastSequenceNumber(),
	}, nil
}

func metadataVersion(key string) (int64, error) {
	name := path.Base(key)
	versionPart, _, found := strings.Cut(name, "-")
	if !found {
		return 0, errors.Newf("metadata filename %q has no version", name)
	}
	version, err := strconv.ParseInt(versionPart, 10, 64)
	if err != nil || version < 0 {
		return 0, errors.Newf("metadata filename %q has invalid version", name)
	}
	return version, nil
}

func toObservedMetadataFiles(files []metadataFile) []map[string]any {
	observed := make([]map[string]any, len(files))
	for index, file := range files {
		observed[index] = toObservedMetadataFile(file)
	}
	return observed
}

func toObservedMetadataFile(file metadataFile) map[string]any {
	var currentSnapshot any
	if file.currentSnapshot != nil {
		currentSnapshot = toObservedSnapshot(*file.currentSnapshot)
	}
	snapshots := make([]map[string]any, len(file.snapshots))
	for index, snapshot := range file.snapshots {
		snapshots[index] = toObservedSnapshot(snapshot)
	}
	return map[string]any{
		"location":         file.location,
		"version":          file.version,
		"current_snapshot": currentSnapshot,
		"snapshots":        snapshots,
		"last_sequence_no": file.lastSequenceNo,
	}
}

func toObservedSnapshot(snapshot table.Snapshot) map[string]any {
	var parentSnapshotID any
	if snapshot.ParentSnapshotID != nil {
		parentSnapshotID = *snapshot.ParentSnapshotID
	}
	return map[string]any{
		"sequence_no":        snapshot.SequenceNumber,
		"snapshot_id":        snapshot.SnapshotID,
		"parent_snapshot_id": parentSnapshotID,
		"operation":          strings.ToUpper(string(snapshot.Summary.Operation)),
		"manifest_list_path": snapshot.ManifestList,
	}
}
