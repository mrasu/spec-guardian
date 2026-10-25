package field

import (
	"encoding/json"
	"fmt"
)

// KeyType identifies a supported DynamoDB key_type value.
type KeyType string

const KeyTypeHash KeyType = "HASH"

// UnmarshalJSON rejects values unsupported by the provider.
func (v *KeyType) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode DynamoDB key_type: %w", err)
	}
	value := KeyType(text)
	switch value {
	case KeyTypeHash:
		*v = value
		return nil
	default:
		return fmt.Errorf("unsupported DynamoDB key_type %q", text)
	}
}
