// Package datafile observes all Parquet data files under an Iceberg table's S3 prefix.
package datafiles

import (
	"context"

	"spec-guardian/examples/iceberg/conformance/specguardian/file"
	"spec-guardian/examples/iceberg/conformance/specguardian/provider/common"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/provider"
)

// Config supplies constructor settings. The table and S3 client are set per attempt.
type Config struct{}

// Provider reads every data file in a table prefix, including uncommitted files.
type Provider struct {
	base *common.BaseProvider
}

type dataFile struct {
	filePath string
	rows     []file.IcebergFileParquetRow
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.Observer = (*Provider)(nil)

// New creates the data file provider used by generated conformance tests.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the field requests and creates a data file provider.
func NewProvider(_ Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	base, err := common.NewBaseProvider("data file", config, fields, validateType)
	if err != nil {
		return nil, err
	}
	return &Provider{base: base}, nil
}

func validateType(spec provider.Type) error {
	if spec.Kind != provider.KindList || spec.Values == nil || spec.Values.Kind != provider.KindMessage {
		return errors.New("data file field requires a list of messages")
	}
	fields := spec.Values.Fields
	filePath, hasFilePath := fields["file_path"]
	rows, hasRows := fields["rows"]
	if len(fields) != 2 || !hasFilePath || filePath.Kind != provider.KindString || !hasRows || !isStringRows(rows) {
		return errors.New("data file field requires file_path and rows of string lists")
	}
	return nil
}

func isStringRows(rows provider.Type) bool {
	return rows.Kind == provider.KindList && rows.Values != nil &&
		rows.Values.Kind == provider.KindList && rows.Values.Values != nil &&
		rows.Values.Values.Kind == provider.KindString
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

// Observe reads all data files for the declared field.
func (o *Provider) Observe(ctx context.Context, field string) (any, error) {
	if err := o.base.CheckField(field); err != nil {
		return nil, err
	}
	files, err := o.readDataFiles(ctx)
	if err != nil {
		return nil, err
	}
	return toObservedFiles(files), nil
}

func (o *Provider) readDataFiles(ctx context.Context) ([]dataFile, error) {
	keys, err := o.base.TableObjects().ListKeys(ctx, ".parquet")
	if err != nil {
		return nil, err
	}
	files := make([]dataFile, 0, len(keys))
	for _, key := range keys {
		rows, isDataFile, err := o.readDataFile(ctx, key)
		if err != nil {
			return nil, err
		}
		if !isDataFile {
			continue
		}
		files = append(files, dataFile{filePath: key, rows: rows})
	}

	return files, nil
}

func toObservedFiles(files []dataFile) []map[string]any {
	observed := make([]map[string]any, len(files))
	for index, f := range files {
		rows := make([][]string, len(f.rows))
		for i, row := range f.rows {
			rows[i] = []string{row.Col1, row.Col2, row.Col3}
		}
		observed[index] = map[string]any{"file_path": f.filePath, "rows": rows}
	}
	return observed
}

func (o *Provider) readDataFile(ctx context.Context, key string) ([]file.IcebergFileParquetRow, bool, error) {
	contents, err := o.base.TableObjects().Read(ctx, key)
	if err != nil {
		return nil, false, err
	}
	kind, err := common.DetectParquetFileKind(contents)
	if err != nil {
		return nil, false, err
	}
	if kind == common.ParquetPositionDeleteFile {
		return nil, false, nil
	}
	rows, err := common.ReadParquet[file.IcebergFileParquetRow](contents)
	if err != nil {
		return nil, false, err
	}
	return rows, true, nil
}
