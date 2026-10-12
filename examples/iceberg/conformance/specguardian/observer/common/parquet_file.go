package common

import (
	"bytes"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/parquet-go/parquet-go"
)

// ParquetFileKind identifies the Iceberg file represented by a Parquet schema.
type ParquetFileKind uint8

const (
	ParquetDataFile ParquetFileKind = iota + 1
	ParquetPositionDeleteFile
)

// DetectParquetFileKind classifies a table Parquet object by its root fields.
func DetectParquetFileKind(contents []byte) (ParquetFileKind, error) {
	file, err := parquet.OpenFile(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return 0, errors.Wrap(err, "open Parquet file")
	}
	fields := file.Schema().Fields()
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name())
	}
	slices.Sort(names)
	if slices.Equal(names, []string{"col1", "col2", "col3"}) {
		return ParquetDataFile, nil
	}
	if slices.Equal(names, []string{"file_path", "pos"}) {
		return ParquetPositionDeleteFile, nil
	}
	return 0, errors.Newf("unknown table Parquet schema: %v", names)
}

// ReadParquet reads rows from a classified table Parquet object.
func ReadParquet[T any](contents []byte) ([]T, error) {
	rows, err := parquet.Read[T](bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return nil, errors.Wrap(err, "read Parquet file")
	}
	return rows, nil
}
