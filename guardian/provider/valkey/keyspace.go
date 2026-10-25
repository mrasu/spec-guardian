package valkey

import (
	"fmt"
	"strings"
)

// keyspace declares the scope and stored data type of a group of keys.
type keyspace struct {
	Name     string   `json:"name"`
	Prefix   string   `json:"prefix"`
	DataType dataType `json:"data_type"`
}

func resolveKeyspaces(declarations []keyspace) (map[string]keyspace, error) {
	keyspaces := make(map[string]keyspace, len(declarations))
	for _, space := range declarations {
		if _, exists := keyspaces[space.Name]; exists {
			return nil, fmt.Errorf("duplicate Valkey keyspace %q", space.Name)
		}
		keyspaces[space.Name] = space
	}
	return keyspaces, nil
}

// fieldScope contains the resolved key selection and model-key mapping.
type fieldScope struct {
	Pattern string
	Prefix  string
	Seed    bool
}

func resolveFieldScope(field fieldRequest, keyspaces map[string]keyspace) (fieldScope, error) {
	if name := field.Config.Keyspace; name != "" {
		space, exists := keyspaces[name]
		if !exists {
			return fieldScope{}, fmt.Errorf("Valkey field %s references unknown keyspace %q", field.Field, name)
		}
		return fieldScope{Pattern: escapePattern(space.Prefix) + "*", Prefix: space.Prefix, Seed: true}, nil
	}
	return fieldScope{Pattern: field.Config.Keys}, nil
}

// escapePattern preserves a literal prefix when constructing SCAN MATCH patterns.
func escapePattern(prefix string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`, "]", `\]`).Replace(prefix)
}
