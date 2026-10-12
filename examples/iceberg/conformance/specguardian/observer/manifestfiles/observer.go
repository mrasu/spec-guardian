// Package manifestfiles observes Iceberg manifest files in S3.
package manifestfiles

import (
	"bytes"
	"context"
	"path"
	"reflect"
	"strconv"
	"strings"

	"spec-guardian/examples/iceberg/conformance/specguardian/observer/common"

	iceberggo "github.com/apache/iceberg-go"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/observer"
	"github.com/twmb/avro/ocf"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Observer reads every manifest file under one table prefix.
type Observer struct {
	base *common.BaseObserver
}

type manifestFile struct {
	location string
	content  iceberggo.ManifestContent
	entries  []iceberggo.ManifestEntry
}

type manifestHeader struct {
	version int
	content iceberggo.ManifestContent
	specID  int32
}

var _ observer.Observer = (*Observer)(nil)

var manifestFilesType = observer.Type{Kind: observer.KindList, Values: &observer.Type{
	Kind: observer.KindMessage,
	Fields: map[string]observer.Type{
		"location": {Kind: observer.KindString},
		"file": {Kind: observer.KindMessage, Fields: map[string]observer.Type{
			"content": {Kind: observer.KindString},
			"entries": {Kind: observer.KindList, Values: &observer.Type{
				Kind: observer.KindMessage,
				Fields: map[string]observer.Type{
					"status":           {Kind: observer.KindString},
					"data_sequence_no": {Kind: observer.KindInt64},
					"file_sequence_no": {Kind: observer.KindInt64},
					"snapshot_id":      {Kind: observer.KindInt64},
					"data_file": {Kind: observer.KindMessage, Fields: map[string]observer.Type{
						"content":    {Kind: observer.KindString},
						"file_index": {Kind: observer.KindInt64},
					}},
				},
			}},
			"snapshot_id":         {Kind: observer.KindInt64},
			"sequence_no":         {Kind: observer.KindInt64},
			"min_sequence_no":     {Kind: observer.KindInt64},
			"added_file_count":    {Kind: observer.KindInt64},
			"deleted_file_count":  {Kind: observer.KindInt64},
			"existing_file_count": {Kind: observer.KindInt64},
		}},
	},
}}

// New creates the manifest file observer used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the field requests and creates a manifest file observer.
func NewObserver(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	base, err := common.NewBaseObserver("manifest file", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Observer{base: base}, nil
}

func validateType(spec observer.Type) error {
	if !reflect.DeepEqual(spec, manifestFilesType) {
		return errors.New("manifest file field requires location and file contents")
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

// Observe returns every manifest file in the table prefix.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	files, err := o.readManifestFiles(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedManifestFiles(files), nil
}

func (o *Observer) readManifestFiles(ctx context.Context) ([]manifestFile, error) {
	keys, err := o.base.TableObjects().ListKeys(ctx, ".avro")
	if err != nil {
		return nil, err
	}
	files := []manifestFile{}
	for _, key := range keys {
		if strings.HasPrefix(path.Base(key), "snap-") {
			continue
		}
		file, err := o.readManifestFile(ctx, key)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, nil
}

func (o *Observer) readManifestFile(ctx context.Context, key string) (manifestFile, error) {
	contents, err := o.base.TableObjects().Read(ctx, key)
	if err != nil {
		return manifestFile{}, err
	}
	header, err := readManifestHeader(contents)
	if err != nil {
		return manifestFile{}, err
	}
	descriptor := iceberggo.NewManifestFile(header.version, key, int64(len(contents)), header.specID, 0).
		Content(header.content).SequenceNum(0, 0).Build()
	entries, err := iceberggo.ReadManifest(descriptor, bytes.NewReader(contents), false)
	if err != nil {
		return manifestFile{}, errors.Wrapf(err, "read manifest file %s", key)
	}
	return manifestFile{location: key, content: header.content, entries: entries}, nil
}

func readManifestHeader(contents []byte) (manifestHeader, error) {
	reader, err := ocf.NewReader(bytes.NewReader(contents))
	if err != nil {
		return manifestHeader{}, errors.Wrap(err, "read manifest header")
	}
	metadata := reader.Metadata()
	if err := reader.Close(); err != nil {
		return manifestHeader{}, errors.Wrap(err, "close manifest header")
	}
	version := 1
	if raw := metadata["format-version"]; len(raw) != 0 {
		version, err = strconv.Atoi(string(raw))
		if err != nil {
			return manifestHeader{}, errors.Wrap(err, "parse manifest format version")
		}
	}
	content := iceberggo.ManifestContentData
	switch string(metadata["content"]) {
	case "", "data":
	case "deletes":
		content = iceberggo.ManifestContentDeletes
	default:
		return manifestHeader{}, errors.Newf("invalid manifest content %q", metadata["content"])
	}
	specID, err := strconv.ParseInt(string(metadata["partition-spec-id"]), 10, 32)
	if err != nil {
		return manifestHeader{}, errors.Wrap(err, "parse manifest partition spec ID")
	}
	return manifestHeader{version: version, content: content, specID: int32(specID)}, nil
}

func toObservedManifestFiles(files []manifestFile) []map[string]any {
	observed := make([]map[string]any, len(files))
	for index, file := range files {
		observed[index] = toObservedManifestFile(file)
	}
	return observed
}

func toObservedManifestFile(file manifestFile) map[string]any {
	entries := make([]map[string]any, len(file.entries))
	var added, deleted, existing int64
	for index, entry := range file.entries {
		entries[index] = toObservedManifestEntry(entry)
		switch entry.Status() {
		case iceberggo.EntryStatusADDED:
			added++
		case iceberggo.EntryStatusDELETED:
			deleted++
		case iceberggo.EntryStatusEXISTING:
			existing++
		}
	}
	return map[string]any{
		"location": file.location,
		"file": map[string]any{
			"content":             strings.ToUpper(file.content.String()),
			"entries":             entries,
			"snapshot_id":         int64(0),
			"sequence_no":         int64(0),
			"min_sequence_no":     int64(0),
			"added_file_count":    added,
			"deleted_file_count":  deleted,
			"existing_file_count": existing,
		},
	}
}

func toObservedManifestEntry(entry iceberggo.ManifestEntry) map[string]any {
	status := ""
	switch entry.Status() {
	case iceberggo.EntryStatusADDED:
		status = "ADDED"
	case iceberggo.EntryStatusDELETED:
		status = "DELETED"
	case iceberggo.EntryStatusEXISTING:
		status = "EXISTING"
	}
	content := ""
	switch entry.DataFile().ContentType() {
	case iceberggo.EntryContentData:
		content = "DATA"
	case iceberggo.EntryContentPosDeletes:
		content = "POS_DELETE"
	case iceberggo.EntryContentEqDeletes:
		content = "EQ_DELETE"
	}
	return map[string]any{
		"status":           status,
		"data_sequence_no": entry.SequenceNum(),
		"file_sequence_no": int64(0),
		"snapshot_id":      entry.SnapshotID(),
		"data_file": map[string]any{
			"content":    content,
			"file_index": int64(0),
		},
	}
}
