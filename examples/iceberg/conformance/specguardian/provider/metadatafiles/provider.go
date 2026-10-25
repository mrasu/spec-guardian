// Package metadatafiles observes Iceberg table metadata files in S3.
package metadatafiles

import (
	"context"
	"path"
	"reflect"
	"strconv"
	"strings"

	"spec-guardian/examples/iceberg/conformance/specguardian/provider/common"

	"github.com/apache/iceberg-go/table"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/provider"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Provider reads all metadata files under one table prefix.
type Provider struct {
	base *common.BaseProvider
}

type metadataFile struct {
	location        string
	version         int64
	currentSnapshot *table.Snapshot
	snapshots       []table.Snapshot
	lastSequenceNo  int64
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.Observer = (*Provider)(nil)

var (
	snapshotType = provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{
		"sequence_no":        {Kind: provider.KindInt64},
		"snapshot_id":        {Kind: provider.KindInt64},
		"parent_snapshot_id": {Kind: provider.KindInt64, Nullable: true},
		"operation":          {Kind: provider.KindString},
		"manifest_list_path": {Kind: provider.KindString},
	}}
	metadataFilesType = provider.Type{Kind: provider.KindList, Values: &provider.Type{
		Kind: provider.KindMessage,
		Fields: map[string]provider.Type{
			"location":         {Kind: provider.KindString},
			"version":          {Kind: provider.KindInt64},
			"current_snapshot": {Kind: provider.KindMessage, Nullable: true, Fields: snapshotType.Fields},
			"snapshots":        {Kind: provider.KindList, Values: &snapshotType},
			"last_sequence_no": {Kind: provider.KindInt64},
		},
	}}
)

// New creates the metadata file provider used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the field requests and creates a metadata file provider.
func NewProvider(_ Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	base, err := common.NewBaseProvider("metadata file", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Provider{base: base}, nil
}

func validateType(spec provider.Type) error {
	if !reflect.DeepEqual(spec, metadataFilesType) {
		return errors.New("metadata file field requires location, version, current_snapshot, snapshots, and last_sequence_no")
	}
	return nil
}

// SetTable supplies the storage location for the next attempt.
func (o *Provider) SetTable(client *s3.Client, bucket, prefix string) error {
	return o.base.SetTable(client, bucket, prefix)
}

// PrepareCase begins a conformance case.
func (o *Provider) PrepareCase(ctx context.Context) error {
	return o.base.PrepareCase(ctx)
}

// PrepareAttempt checks that setup supplied a table.
func (o *Provider) PrepareAttempt(ctx context.Context) error {
	return o.base.PrepareAttempt(ctx)
}

// CleanupAttempt clears the table assigned to the attempt.
func (o *Provider) CleanupAttempt(ctx context.Context) error {
	return o.base.CleanupAttempt(ctx)
}

// CleanupCase ends a conformance case.
func (o *Provider) CleanupCase(ctx context.Context) error {
	return o.base.CleanupCase(ctx)
}

// Observe returns every metadata file in the table prefix.
func (o *Provider) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	files, err := o.readMetadataFiles(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedMetadataFiles(files), nil
}

func (o *Provider) readMetadataFiles(ctx context.Context) ([]metadataFile, error) {
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

func (o *Provider) readMetadataFile(ctx context.Context, key string) (metadataFile, error) {
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
