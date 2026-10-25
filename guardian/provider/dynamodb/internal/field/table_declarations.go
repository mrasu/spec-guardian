package field

import (
	"fmt"
)

// AttributeDefinition declares an attribute used by a DynamoDB key.
type AttributeDefinition struct {
	AttributeName string        `json:"attribute_name"`
	AttributeType AttributeType `json:"attribute_type"`
}

// KeySchemaElement declares a partition or sort key attribute.
type KeySchemaElement struct {
	AttributeName string  `json:"attribute_name"`
	KeyType       KeyType `json:"key_type"`
}

// Table declares the key structure of an existing DynamoDB table.
// Non-key attributes are described only by the model's field types.
type Table struct {
	Name                 string                `json:"name"`
	AttributeDefinitions []AttributeDefinition `json:"attribute_definitions"`
	KeySchema            []KeySchemaElement    `json:"key_schema"`
}

// ResolveTables checks references and indexes schema-validated table declarations.
func ResolveTables(declarations []Table) (map[string]Table, error) {
	tables := make(map[string]Table, len(declarations))
	for _, table := range declarations {
		if table.AttributeDefinitions[0].AttributeName != table.KeySchema[0].AttributeName {
			return nil, fmt.Errorf("DynamoDB table %q requires a matching string key attribute definition", table.Name)
		}
		if _, exists := tables[table.Name]; exists {
			return nil, fmt.Errorf("duplicate DynamoDB table %q", table.Name)
		}
		tables[table.Name] = table
	}
	return tables, nil
}
