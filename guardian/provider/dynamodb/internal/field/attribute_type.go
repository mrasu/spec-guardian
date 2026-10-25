package field

import (
	"encoding/json"
	"fmt"
)

// AttributeType identifies a supported DynamoDB attribute_type value.
type AttributeType string

const AttributeTypeString AttributeType = "S"

// UnmarshalJSON rejects values unsupported by the provider.
func (v *AttributeType) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode DynamoDB attribute_type: %w", err)
	}
	value := AttributeType(text)
	switch value {
	case AttributeTypeString:
		*v = value
		return nil
	default:
		return fmt.Errorf("unsupported DynamoDB attribute_type %q", text)
	}
}
