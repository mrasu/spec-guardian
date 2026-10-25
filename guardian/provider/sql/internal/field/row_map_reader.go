package field

import (
	"database/sql"
	"fmt"
	"maps"
	"strconv"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

// RowMapReader maps SQL rows to messages keyed by one column.
type RowMapReader struct {
	keyColumn string
	fields    map[string]provider.Type
}

// NewRowMapReader checks schema-validated row_map settings against the model type.
func NewRowMapReader(config ResultConfig, spec provider.Type) (*RowMapReader, error) {
	if spec.Kind != provider.KindMap || spec.Values == nil || spec.Values.Kind != provider.KindMessage {
		return nil, fmt.Errorf("result.type row_map requires map of messages")
	}
	key := config.KeyColumn
	if _, exists := spec.Values.Fields[key]; exists {
		return nil, fmt.Errorf("key column %q cannot also be a message field", key)
	}
	if len(spec.Values.Fields) == 0 {
		return nil, fmt.Errorf("result.type row_map requires message fields")
	}
	for name, field := range spec.Values.Fields {
		if name == "" || field.Kind != provider.KindString || field.Values != nil || len(field.Fields) != 0 {
			return nil, fmt.Errorf("row_map field %q requires string type", name)
		}
	}
	return &RowMapReader{keyColumn: key, fields: maps.Clone(spec.Values.Fields)}, nil
}

func (r *RowMapReader) Read(rows *sql.Rows) (any, error) {
	indexes, err := r.columnIndexes(rows)
	if err != nil {
		return nil, err
	}
	return r.readRows(rows, indexes)
}

func (r *RowMapReader) columnIndexes(rows *sql.Rows) (map[string]int, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	indexes := make(map[string]int, len(columns))
	for index, column := range columns {
		if _, exists := indexes[column]; exists {
			return nil, fmt.Errorf("duplicate column %q", column)
		}
		indexes[column] = index
	}
	if _, exists := indexes[r.keyColumn]; !exists {
		return nil, fmt.Errorf("missing key column %q", r.keyColumn)
	}
	for name := range r.fields {
		if _, exists := indexes[name]; !exists {
			return nil, fmt.Errorf("missing message column %q", name)
		}
	}
	if len(indexes) != len(r.fields)+1 {
		return nil, fmt.Errorf("query has extra columns")
	}
	return indexes, nil
}

func (r *RowMapReader) readRows(rows *sql.Rows, indexes map[string]int) (map[string]any, error) {
	result := make(map[string]any)
	for rows.Next() {
		key, message, err := r.readRow(rows, indexes)
		if err != nil {
			return nil, err
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		result[key] = message
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RowMapReader) readRow(rows *sql.Rows, indexes map[string]int) (string, map[string]any, error) {
	values := make([]any, len(indexes))
	pointers := make([]any, len(values))
	for index := range values {
		pointers[index] = &values[index]
	}
	if err := rows.Scan(pointers...); err != nil {
		return "", nil, err
	}
	key, err := toRowKey(values[indexes[r.keyColumn]])
	if err != nil {
		return "", nil, err
	}
	message, err := r.message(key, values, indexes)
	if err != nil {
		return "", nil, err
	}
	return key, message, nil
}

func (r *RowMapReader) message(key string, values []any, indexes map[string]int) (map[string]any, error) {
	message := make(map[string]any, len(r.fields))
	for name, field := range r.fields {
		value := values[indexes[name]]
		if value == nil {
			if !field.Nullable {
				return nil, fmt.Errorf("row %q column %q is null", key, name)
			}
			message[name] = nil
			continue
		}
		text, err := toString(value)
		if err != nil {
			return nil, fmt.Errorf("row %q column %q: %w", key, name, err)
		}
		message[name] = text
	}
	return message, nil
}

func toRowKey(value any) (string, error) {
	switch value := value.(type) {
	case int64:
		return strconv.FormatInt(value, 10), nil
	case uint64:
		return strconv.FormatUint(value, 10), nil
	case string, []byte:
		return toString(value)
	default:
		return "", fmt.Errorf("key has unsupported type %T", value)
	}
}
