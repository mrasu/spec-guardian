// Package deletefiles observes Iceberg position delete files in S3.
package deletefiles

import (
	"context"
	"reflect"

	"spec-guardian/examples/iceberg/conformance/specguardian/file"
	"spec-guardian/examples/iceberg/conformance/specguardian/observer/common"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/observer"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Observer reads every position delete file under one table prefix.
type Observer struct {
	base *common.BaseObserver
}

type deleteFile struct {
	location string
	rows     []file.PositionDeleteRow
}

var _ observer.Observer = (*Observer)(nil)

var deleteFilesType = observer.Type{Kind: observer.KindList, Values: &observer.Type{
	Kind: observer.KindMessage,
	Fields: map[string]observer.Type{
		"location": {Kind: observer.KindString},
		"file": {Kind: observer.KindList, Values: &observer.Type{
			Kind: observer.KindMessage,
			Fields: map[string]observer.Type{
				"file_index": {Kind: observer.KindInt64},
				"row_index":  {Kind: observer.KindInt64},
			},
		}},
	},
}}

// New creates the delete file observer used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the field requests and creates a delete file observer.
func NewObserver(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	base, err := common.NewBaseObserver("delete file", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Observer{base: base}, nil
}

func validateType(spec observer.Type) error {
	if !reflect.DeepEqual(spec, deleteFilesType) {
		return errors.New("delete file field requires location and file contents")
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

// Observe returns every position delete file in the table prefix.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	files, err := o.readDeleteFiles(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedDeleteFiles(files), nil
}

func (o *Observer) readDeleteFiles(ctx context.Context) ([]deleteFile, error) {
	keys, err := o.base.TableObjects().ListKeys(ctx, ".parquet")
	if err != nil {
		return nil, err
	}
	files := []deleteFile{}
	for _, key := range keys {
		file, isDeleteFile, err := o.readDeleteFile(ctx, key)
		if err != nil {
			return nil, err
		}
		if isDeleteFile {
			files = append(files, file)
		}
	}
	return files, nil
}

func (o *Observer) readDeleteFile(ctx context.Context, key string) (deleteFile, bool, error) {
	contents, err := o.base.TableObjects().Read(ctx, key)
	if err != nil {
		return deleteFile{}, false, err
	}
	kind, err := common.DetectParquetFileKind(contents)
	if err != nil {
		return deleteFile{}, false, err
	}
	if kind == common.ParquetDataFile {
		return deleteFile{}, false, nil
	}
	rows, err := common.ReadParquet[file.PositionDeleteRow](contents)
	if err != nil {
		return deleteFile{}, false, err
	}
	return deleteFile{location: key, rows: rows}, true, nil
}

func toObservedDeleteFiles(files []deleteFile) []map[string]any {
	observed := make([]map[string]any, len(files))
	for index, file := range files {
		rows := make([]map[string]any, len(file.rows))
		for rowIndex, row := range file.rows {
			rows[rowIndex] = map[string]any{
				"file_index": int64(0),
				"row_index":  row.Pos,
			}
		}
		observed[index] = map[string]any{"location": file.location, "file": rows}
	}
	return observed
}
