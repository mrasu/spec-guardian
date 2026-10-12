// Package configschema validates SpecGuardian configuration against its JSON Schema.
package configschema

import (
	_ "embed"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

//go:embed spec-guardian.schema.yaml
var specGuardianSchema []byte

var compiledSpecGuardianSchema = sync.OnceValues(compileSpecGuardianSchema)

// Validate checks a YAML document against the SpecGuardian configuration schema.
func Validate(node *yaml.Node) error {
	var value any
	if err := node.Decode(&value); err != nil {
		return errors.Wrap(err, "decode configuration for validation")
	}
	schema, err := compiledSpecGuardianSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return errors.Wrap(err, "validate configuration")
	}
	return nil
}

func compileSpecGuardianSchema() (*jsonschema.Schema, error) {
	var document any
	if err := yaml.Unmarshal(specGuardianSchema, &document); err != nil {
		return nil, errors.Wrap(err, "read configuration schema")
	}
	compiler := jsonschema.NewCompiler()
	const location = "urn:spec-guardian:configuration"
	if err := compiler.AddResource(location, document); err != nil {
		return nil, errors.Wrap(err, "register configuration schema")
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		return nil, errors.Wrap(err, "compile configuration schema")
	}
	return schema, nil
}
