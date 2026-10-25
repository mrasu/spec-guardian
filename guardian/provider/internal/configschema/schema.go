// Package configschema validates and decodes built-in provider settings.
package configschema

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

const (
	providerConfigFragment = "#/$defs/providerConfig"
	fieldConfigFragment    = "#/$defs/fieldConfig"
)

// Schemas holds the shared and field configuration schemas for one provider.
type Schemas struct {
	provider *jsonschema.Schema
	field    *jsonschema.Schema
}

// NewSchemas compiles the providerConfig and fieldConfig definitions in a YAML schema.
func NewSchemas(data []byte, location string) (*Schemas, error) {
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("read config schema %s: %w", location, err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(location, document); err != nil {
		return nil, fmt.Errorf("register config schema %s: %w", location, err)
	}
	shared, err := compiler.Compile(location + providerConfigFragment)
	if err != nil {
		return nil, fmt.Errorf("compile provider config schema %s: %w", location, err)
	}
	field, err := compiler.Compile(location + fieldConfigFragment)
	if err != nil {
		return nil, fmt.Errorf("compile field config schema %s: %w", location, err)
	}
	return &Schemas{provider: shared, field: field}, nil
}

// DecodeProvider validates shared settings and decodes them into target.
func (s *Schemas) DecodeProvider(settings map[string]any, target any) error {
	return decodeSettings(s.provider, settings, target)
}

// DecodeField validates field settings and decodes them into target.
func (s *Schemas) DecodeField(settings map[string]any, target any) error {
	return decodeSettings(s.field, settings, target)
}

// decodeSettings validates a JSON snapshot and decodes it into internal settings.
// Omitted settings are treated as an empty object.
func decodeSettings(schema *jsonschema.Schema, settings map[string]any, target any) error {
	if settings == nil {
		settings = map[string]any{}
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode settings for validation: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode settings for validation: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode validated settings: %w", err)
	}
	return nil
}
