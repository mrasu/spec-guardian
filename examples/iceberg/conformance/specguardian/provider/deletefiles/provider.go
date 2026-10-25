// Package deletefiles observes Iceberg position delete files in S3.
package deletefiles

import (
	"context"
	"reflect"

	"spec-guardian/examples/iceberg/conformance/specguardian/file"
	"spec-guardian/examples/iceberg/conformance/specguardian/provider/common"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/provider"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Provider reads every position delete file under one table prefix.
type Provider struct {
	base *common.BaseProvider
}

type deleteFile struct {
	location string
	rows     []file.PositionDeleteRow
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.Observer = (*Provider)(nil)

var deleteFilesType = provider.Type{Kind: provider.KindList, Values: &provider.Type{
	Kind: provider.KindMessage,
	Fields: map[string]provider.Type{
		"location": {Kind: provider.KindString},
		"file": {Kind: provider.KindList, Values: &provider.Type{
			Kind: provider.KindMessage,
			Fields: map[string]provider.Type{
				"file_index": {Kind: provider.KindInt64},
				"row_index":  {Kind: provider.KindInt64},
			},
		}},
	},
}}

// New creates the delete file provider used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the field requests and creates a delete file provider.
func NewProvider(_ Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	base, err := common.NewBaseProvider("delete file", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Provider{base: base}, nil
}

func validateType(spec provider.Type) error {
	if !reflect.DeepEqual(spec, deleteFilesType) {
		return errors.New("delete file field requires location and file contents")
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

// Observe returns every position delete file in the table prefix.
func (o *Provider) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	files, err := o.readDeleteFiles(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedDeleteFiles(files), nil
}

func (o *Provider) readDeleteFiles(ctx context.Context) ([]deleteFile, error) {
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

func (o *Provider) readDeleteFile(ctx context.Context, key string) (deleteFile, bool, error) {
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
