package field

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

// TableField prepares and observes a full table using one validated column mapping.
type TableField struct {
	name    string
	driver  string
	columns []columnBinding
}

type columnBinding struct {
	column string
	name   string
	spec   provider.Type
}

// ColumnMapping overrides the default column name for one model field.
type ColumnMapping struct {
	Field  string `json:"field"`
	Column string `json:"column"`
}

// NewTableField resolves schema-validated settings against the model type and tables.
func NewTableField(request Request, tables map[string]Table, driver string) (*TableField, error) {
	config := request.Config
	name := config.Table
	table, exists := tables[name]
	if !exists {
		return nil, fmt.Errorf("unknown SQL table %q", name)
	}
	columns, err := toColumnBindings(request.Output.Type, table, config.Columns)
	if err != nil {
		return nil, err
	}
	return &TableField{name: table.Name, driver: driver, columns: columns}, nil
}

// TableName returns the table assigned to this field.
func (f *TableField) TableName() string { return f.name }

// Observe returns all rows, including duplicates and unexpected rows.
func (f *TableField) Observe(ctx context.Context, db *sql.DB) (any, error) {
	columns := make([]string, len(f.columns))
	for i, column := range f.columns {
		columns[i] = quoteIdentifier(column.column, f.driver)
	}
	query := "SELECT " + strings.Join(columns, ", ") + " FROM " + quoteIdentifier(f.name, f.driver)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return f.readRows(rows)
}

func (f *TableField) readRows(rows *sql.Rows) (any, error) {
	result := make([]map[string]any, 0)
	for rows.Next() {
		message, err := f.readRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (f *TableField) readRow(rows *sql.Rows) (map[string]any, error) {
	values := make([]any, len(f.columns))
	pointers := make([]any, len(values))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return nil, err
	}
	message := make(map[string]any, len(values))
	for i, field := range f.columns {
		converted, err := toScalar(values[i], field.spec)
		if err != nil {
			return nil, fmt.Errorf("column for field %q: %w", field.name, err)
		}
		message[field.name] = converted
	}
	return message, nil
}

// Clear removes all rows in the observed table, including unexpected rows.
func (f *TableField) Clear(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(f.name, f.driver))
	return err
}

// Seed inserts rows in input order, preserving duplicates and explicit nulls.
func (f *TableField) Seed(ctx context.Context, db *sql.DB, value any) (any, error) {
	rows, err := provider.Convert[[]map[string]any](value)
	if err != nil {
		return nil, err
	}
	for i, row := range rows {
		if err := f.insertRow(ctx, db, row); err != nil {
			return nil, fmt.Errorf("row %d: %w", i, err)
		}
	}
	return value, nil
}

func (f *TableField) insertRow(ctx context.Context, db *sql.DB, row map[string]any) error {
	args, err := f.arguments(row)
	if err != nil {
		return err
	}
	columns := make([]string, len(f.columns))
	placeholders := make([]string, len(columns))
	for i, column := range f.columns {
		columns[i] = quoteIdentifier(column.column, f.driver)
		placeholders[i] = "?"
		if f.driver == "pgx" || f.driver == "postgres" {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
	}
	query := "INSERT INTO " + quoteIdentifier(f.name, f.driver) +
		" (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	_, err = db.ExecContext(ctx, query, args...)
	return err
}

func (f *TableField) arguments(row map[string]any) ([]any, error) {
	if len(row) != len(f.columns) {
		return nil, fmt.Errorf("row fields must match table columns")
	}
	args := make([]any, len(f.columns))
	for i, field := range f.columns {
		value, exists := row[field.name]
		if !exists {
			return nil, fmt.Errorf("missing field %q", field.name)
		}
		scalar, err := toSeedValue(value, field.spec)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.name, err)
		}
		args[i] = scalar
	}
	return args, nil
}

func toColumnBindings(spec provider.Type, table Table, mappings []ColumnMapping) ([]columnBinding, error) {
	if spec.Kind != provider.KindList || spec.Values == nil || spec.Values.Kind != provider.KindMessage {
		return nil, fmt.Errorf("SQL table field requires a list of messages")
	}
	fields, err := mappedFields(mappings, spec.Values.Fields)
	if err != nil {
		return nil, err
	}
	columns := make([]columnBinding, 0, len(table.Columns))
	for _, column := range table.Columns {
		fieldName := fields[column.Name]
		field, exists := spec.Values.Fields[fieldName]
		if !exists || field.Kind != column.Kind || field.Values != nil || len(field.Fields) != 0 {
			return nil, fmt.Errorf("SQL column %q requires a matching message field of type %s", column.Name, column.Kind)
		}
		columns = append(columns, columnBinding{column: column.Name, name: fieldName, spec: field})
	}
	if len(columns) != len(spec.Values.Fields) {
		return nil, fmt.Errorf("SQL table columns and message fields must match")
	}
	return columns, nil
}

func mappedFields(mappings []ColumnMapping, fields map[string]provider.Type) (map[string]string, error) {
	overrides := make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		_, known := fields[mapping.Field]
		if !known || overrides[mapping.Field] != "" {
			return nil, fmt.Errorf("invalid SQL column mapping for field %q", mapping.Field)
		}
		overrides[mapping.Field] = mapping.Column
	}
	columns := make(map[string]string, len(fields))
	for name := range fields {
		column := name
		if override, exists := overrides[name]; exists {
			column = override
		}
		if _, exists := columns[column]; exists {
			return nil, fmt.Errorf("multiple fields map to column %q", column)
		}
		columns[column] = name
	}
	return columns, nil
}

func quoteIdentifier(name, driver string) string {
	quote := `"`
	if driver == "mysql" {
		quote = "`"
	}
	return quote + strings.ReplaceAll(name, quote, quote+quote) + quote
}
