package dynamodb

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/mrasu/spec-guardian/guardian/provider"
	dynamofield "github.com/mrasu/spec-guardian/guardian/provider/dynamodb/internal/field"
	"github.com/mrasu/spec-guardian/guardian/provider/internal/configschema"
)

const configSchemaLocation = "urn:spec-guardian:provider:dynamodb"

//go:embed config.schema.yaml
var configSchema []byte

var compiledConfigSchemas = sync.OnceValues(func() (*configschema.Schemas, error) {
	return configschema.NewSchemas(configSchema, configSchemaLocation)
})

// parseConfig validates and decodes settings for internal use.
func parseConfig(config map[string]any, fields []provider.FieldRequest) (*providerConfig, error) {
	schemas, err := compiledConfigSchemas()
	if err != nil {
		return nil, err
	}
	var parsed providerConfig
	if err := schemas.DecodeProvider(config, &parsed); err != nil {
		return nil, fmt.Errorf("dynamodb provider config: %w", err)
	}
	for _, field := range fields {
		var settings dynamofield.Config
		if err := schemas.DecodeField(field.Config, &settings); err != nil {
			return nil, fmt.Errorf("dynamodb field %s config: %w", field.Field, err)
		}
		parsed.Fields = append(parsed.Fields, dynamofield.Request{Field: field.Field, Output: field.Output, Config: settings})
	}
	return &parsed, nil
}
