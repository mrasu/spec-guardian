package fizzbee

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ValueKind identifies a JSON value representation.
type ValueKind uint8

const (
	ValueKindNull ValueKind = iota
	ValueKindBool
	ValueKindInteger
	ValueKindFloat
	ValueKindString
	ValueKindArray
	ValueKindObject
	ValueKindInvalid
)

// JSONObject is a validated JSON object.
type JSONObject map[string]any

// NewJSONObject parses raw as a JSON object.
func NewJSONObject(raw json.RawMessage) (JSONObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("got %T, want object", value)
	}
	return JSONObject(object), nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("multiple JSON values")
	}
	return err
}

// ValueKindOf reports the JSON representation of value.
func ValueKindOf(value any) ValueKind {
	switch value := value.(type) {
	case nil:
		return ValueKindNull
	case bool:
		return ValueKindBool
	case json.Number:
		if strings.ContainsAny(value.String(), ".eE") {
			return ValueKindFloat
		}
		return ValueKindInteger
	case string:
		return ValueKindString
	case []any:
		return ValueKindArray
	case map[string]any:
		return ValueKindObject
	default:
		return ValueKindInvalid
	}
}
