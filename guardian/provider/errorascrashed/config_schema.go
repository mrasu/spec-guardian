package errorascrashed

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/mrasu/spec-guardian/guardian/provider/internal/configschema"
)

const configSchemaLocation = "urn:spec-guardian:provider:errorascrashed"

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
		return nil, fmt.Errorf("errorascrashed provider config: %w", err)
	}
	for _, field := range fields {
		var settings struct{}
		if err := schemas.DecodeField(field.Config, &settings); err != nil {
			return nil, fmt.Errorf("errorascrashed field %s config: %w", field.Field, err)
		}
		parsed.Fields = append(parsed.Fields, fieldRequest{Field: field.Field, Output: field.Output})
	}
	return &parsed, nil
}
