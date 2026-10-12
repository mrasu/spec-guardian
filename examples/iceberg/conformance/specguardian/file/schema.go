package file

import iceberggo "github.com/apache/iceberg-go"

var IcebergSchema = iceberggo.NewSchema(0,
	iceberggo.NestedField{ID: 1, Name: "col1", Type: iceberggo.PrimitiveTypes.String, Required: true},
	iceberggo.NestedField{ID: 2, Name: "col2", Type: iceberggo.PrimitiveTypes.String, Required: true},
	iceberggo.NestedField{ID: 3, Name: "col3", Type: iceberggo.PrimitiveTypes.String, Required: true},
)

type IcebergFileParquetRow struct {
	Col1 string `parquet:"col1"`
	Col2 string `parquet:"col2"`
	Col3 string `parquet:"col3"`
}

// PositionDeleteRow is the Parquet representation of a position delete.
type PositionDeleteRow struct {
	FilePath string `parquet:"file_path"`
	Pos      int64  `parquet:"pos"`
}
