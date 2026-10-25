package field

import (
	"database/sql"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

// KVReader maps SQL rows to a map using two named columns.
type KVReader struct {
	keyColumn   string
	valueColumn string
}

// NewKVReader checks the model type for schema-validated kv settings.
func NewKVReader(config ResultConfig, spec provider.Type) (*KVReader, error) {
	if spec.Kind != provider.KindMap || spec.Values == nil || spec.Values.Kind != provider.KindString {
		return nil, fmt.Errorf("result.type kv requires map of strings")
	}
	return &KVReader{keyColumn: config.KeyColumn, valueColumn: config.ValueColumn}, nil
}

func (r *KVReader) Read(rows *sql.Rows) (any, error) {
	keyIndex, valueIndex, err := r.columnIndexes(rows)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for rows.Next() {
		key, value, err := scanKVRow(rows, keyIndex, valueIndex)
		if err != nil {
			return nil, err
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		result[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *KVReader) columnIndexes(rows *sql.Rows) (int, int, error) {
	columns, err := rows.Columns()
	if err != nil {
		return 0, 0, err
	}
	keyIndex, valueIndex := -1, -1
	for index, column := range columns {
		if column == r.keyColumn {
			keyIndex = index
		}
		if column == r.valueColumn {
			valueIndex = index
		}
	}
	if keyIndex < 0 || valueIndex < 0 || keyIndex == valueIndex {
		return 0, 0, fmt.Errorf("key/value columns not found in query")
	}
	return keyIndex, valueIndex, nil
}

func scanKVRow(rows *sql.Rows, keyIndex, valueIndex int) (string, string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return "", "", err
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for index := range values {
		pointers[index] = &values[index]
	}
	if err := rows.Scan(pointers...); err != nil {
		return "", "", err
	}
	key, err := toString(values[keyIndex])
	if err != nil {
		return "", "", fmt.Errorf("key: %w", err)
	}
	value, err := toString(values[valueIndex])
	if err != nil {
		return "", "", fmt.Errorf("value: %w", err)
	}
	return key, value, nil
}
