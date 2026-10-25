package field

import (
	"encoding/json"
	"fmt"
)

// ResultType identifies a supported DynamoDB result.type value.
type ResultType string

const (
	ResultTypeItemMap  ResultType = "item_map"
	ResultTypeItemList ResultType = "item_list"
)

// UnmarshalJSON rejects values unsupported by the provider.
func (v *ResultType) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode DynamoDB result.type: %w", err)
	}
	value := ResultType(text)
	switch value {
	case ResultTypeItemMap, ResultTypeItemList:
		*v = value
		return nil
	default:
		return fmt.Errorf("unsupported DynamoDB result.type %q", text)
	}
}
