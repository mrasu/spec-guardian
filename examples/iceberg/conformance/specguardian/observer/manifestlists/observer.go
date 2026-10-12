// Package manifestlists observes Iceberg manifest list files in S3.
package manifestlists

import (
	"bytes"
	"context"
	"path"
	"reflect"
	"strings"

	"spec-guardian/examples/iceberg/conformance/specguardian/observer/common"

	iceberggo "github.com/apache/iceberg-go"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/observer"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Observer reads every manifest list under one table prefix.
type Observer struct {
	base *common.BaseObserver
}

type manifestList struct {
	location string
	entries  []iceberggo.ManifestFile
}

var _ observer.Observer = (*Observer)(nil)

var manifestListsType = observer.Type{Kind: observer.KindList, Values: &observer.Type{
	Kind: observer.KindMessage,
	Fields: map[string]observer.Type{
		"location": {Kind: observer.KindString},
		"entries": {Kind: observer.KindList, Values: &observer.Type{
			Kind: observer.KindMessage,
			Fields: map[string]observer.Type{
				"manifest_path":       {Kind: observer.KindString},
				"content":             {Kind: observer.KindString},
				"sequence_no":         {Kind: observer.KindInt64},
				"min_sequence_no":     {Kind: observer.KindInt64},
				"snapshot_id":         {Kind: observer.KindInt64},
				"added_file_count":    {Kind: observer.KindInt64},
				"deleted_file_count":  {Kind: observer.KindInt64},
				"existing_file_count": {Kind: observer.KindInt64},
			},
		}},
	},
}}

// New creates the manifest list observer used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the field requests and creates a manifest list observer.
func NewObserver(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	base, err := common.NewBaseObserver("manifest list", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Observer{base: base}, nil
}

func validateType(spec observer.Type) error {
	if !reflect.DeepEqual(spec, manifestListsType) {
		return errors.New("manifest list field requires location and manifest entries")
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

// Observe returns every manifest list in the table prefix.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	lists, err := o.readManifestLists(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedManifestLists(lists), nil
}

func (o *Observer) readManifestLists(ctx context.Context) ([]manifestList, error) {
	keys, err := o.base.TableObjects().ListKeys(ctx, ".avro")
	if err != nil {
		return nil, err
	}
	lists := []manifestList{}
	for _, key := range keys {
		if !strings.HasPrefix(path.Base(key), "snap-") {
			continue
		}
		contents, err := o.base.TableObjects().Read(ctx, key)
		if err != nil {
			return nil, err
		}
		entries, err := iceberggo.ReadManifestList(bytes.NewReader(contents))
		if err != nil {
			return nil, errors.Wrapf(err, "read manifest list %s", key)
		}
		lists = append(lists, manifestList{location: key, entries: entries})
	}

	return lists, nil
}

func toObservedManifestLists(lists []manifestList) []map[string]any {
	observed := make([]map[string]any, len(lists))
	for index, list := range lists {
		entries := make([]map[string]any, len(list.entries))
		for entryIndex, entry := range list.entries {
			entries[entryIndex] = toObservedManifestEntry(entry)
		}
		observed[index] = map[string]any{"location": list.location, "entries": entries}
	}
	return observed
}

func toObservedManifestEntry(entry iceberggo.ManifestFile) map[string]any {
	return map[string]any{
		"manifest_path":       entry.FilePath(),
		"content":             strings.ToUpper(entry.ManifestContent().String()),
		"sequence_no":         entry.SequenceNum(),
		"min_sequence_no":     entry.MinSequenceNum(),
		"snapshot_id":         entry.SnapshotID(),
		"added_file_count":    int64(entry.AddedDataFiles()),
		"deleted_file_count":  int64(entry.DeletedDataFiles()),
		"existing_file_count": int64(entry.ExistingDataFiles()),
	}
}
