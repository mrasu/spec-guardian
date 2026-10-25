package valkey

import (
	"encoding/json"
	"fmt"
)

// dataType identifies a supported Valkey data_type value.
type dataType string

const dataTypeString dataType = "string"

// UnmarshalJSON rejects values unsupported by the provider.
func (v *dataType) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode Valkey data_type: %w", err)
	}
	value := dataType(text)
	switch value {
	case dataTypeString:
		*v = value
		return nil
	default:
		return fmt.Errorf("unsupported Valkey data_type %q", text)
	}
}
