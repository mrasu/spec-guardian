// Package iceberg provides the thin Iceberg API wrapper used by this example.
package iceberg

import (
	"context"

	"github.com/apache/arrow-go/v18/arrow/array"
	iceberggo "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// IcebergGoTable delegates public table operations to an iceberg-go table.
type IcebergGoTable struct {
	table *table.Table
}

// NewIcebergGoTable returns a IcebergGoTable that delegates operations to icebergTable.
func NewIcebergGoTable(icebergTable *table.Table) *IcebergGoTable {
	return &IcebergGoTable{table: icebergTable}
}

// Append delegates to the wrapped iceberg-go table's Append method.
func (t *IcebergGoTable) Append(ctx context.Context, reader array.RecordReader, snapshotProperties iceberggo.Properties) (*table.Table, error) {
	return t.table.Append(ctx, reader, snapshotProperties)
}

// Overwrite delegates to the wrapped iceberg-go table's Overwrite method.
func (t *IcebergGoTable) Overwrite(ctx context.Context, reader array.RecordReader, snapshotProps iceberggo.Properties, opts ...table.OverwriteOption) (*table.Table, error) {
	return t.table.Overwrite(ctx, reader, snapshotProps, opts...)
}

// Delete delegates to the wrapped iceberg-go table's Delete method.
func (t *IcebergGoTable) Delete(ctx context.Context, filter iceberggo.BooleanExpression, snapshotProps iceberggo.Properties, opts ...table.DeleteOption) (*table.Table, error) {
	return t.table.Delete(ctx, filter, snapshotProps, opts...)
}
