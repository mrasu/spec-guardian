package field

import (
	"encoding/json"
	"fmt"
)

// ResultType identifies a supported SQL result.type value.
type ResultType string

const (
	ResultTypeKV     ResultType = "kv"
	ResultTypeRowMap ResultType = "row_map"
)

// UnmarshalJSON rejects values unsupported by the provider.
func (v *ResultType) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode SQL result.type: %w", err)
	}
	value := ResultType(text)
	switch value {
	case ResultTypeKV, ResultTypeRowMap:
		*v = value
		return nil
	default:
		return fmt.Errorf("unsupported SQL result.type %q", text)
	}
}
