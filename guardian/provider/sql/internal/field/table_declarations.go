package field

import (
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

// Column declares one SQL column's stored value type.
type Column struct {
	Name string        `json:"name"`
	Kind provider.Kind `json:"type"`
}

// Table declares the columns observed in an existing SQL table.
type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// ResolveTables indexes schema-validated declarations and checks name uniqueness.
func ResolveTables(declarations []Table) (map[string]Table, error) {
	tables := make(map[string]Table, len(declarations))
	for _, table := range declarations {
		if _, exists := tables[table.Name]; exists {
			return nil, fmt.Errorf("duplicate SQL table %q", table.Name)
		}
		seen := make(map[string]bool, len(table.Columns))
		for _, column := range table.Columns {
			if seen[column.Name] {
				return nil, fmt.Errorf("duplicate column %q", column.Name)
			}
			seen[column.Name] = true
		}
		tables[table.Name] = table
	}
	return tables, nil
}
