package field

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/mrasu/spec-guardian/guardian/provider"
)

func addRecords(records map[string]map[string]any, items []map[string]types.AttributeValue, keyAttribute string, fields map[string]provider.Type) error {
	for _, item := range items {
		key, record, err := toRecord(item, keyAttribute, fields)
		if err != nil {
			return err
		}
		if _, exists := records[key]; exists {
			return fmt.Errorf("duplicate key %q", key)
		}
		records[key] = record
	}
	return nil
}

func toRecord(item map[string]types.AttributeValue, keyAttribute string, fields map[string]provider.Type) (string, map[string]any, error) {
	keyValue, ok := item[keyAttribute].(*types.AttributeValueMemberS)
	if !ok {
		return "", nil, fmt.Errorf("key attribute %q is missing or is not a string", keyAttribute)
	}
	record := make(map[string]any, len(fields))
	for name, spec := range fields {
		attribute, exists := item[name]
		if !exists {
			return "", nil, fmt.Errorf("record %q: missing attribute %q", keyValue.Value, name)
		}
		value, err := toModelValue(attribute, spec)
		if err != nil {
			return "", nil, fmt.Errorf("record %q attribute %q: %w", keyValue.Value, name, err)
		}
		record[name] = value
	}
	for name := range item {
		if name != keyAttribute {
			if _, known := fields[name]; !known {
				return "", nil, fmt.Errorf("record %q: extra attribute %q", keyValue.Value, name)
			}
		}
	}
	return keyValue.Value, record, nil
}

func toModelValue(attribute types.AttributeValue, spec provider.Type) (any, error) {
	if null, ok := attribute.(*types.AttributeValueMemberNULL); ok {
		if spec.Nullable && null.Value {
			return nil, nil
		}
		return nil, fmt.Errorf("NULL requires a nullable field and a true value")
	}
	switch spec.Kind {
	case provider.KindString:
		if text, ok := attribute.(*types.AttributeValueMemberS); ok {
			return text.Value, nil
		}
	case provider.KindInt64:
		if number, ok := attribute.(*types.AttributeValueMemberN); ok {
			value, err := strconv.ParseInt(number.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid int64 %q: %w", number.Value, err)
			}
			return value, nil
		}
	case provider.KindBool:
		if boolean, ok := attribute.(*types.AttributeValueMemberBOOL); ok {
			return boolean.Value, nil
		}
	}
	return nil, fmt.Errorf("got %T for model type %s", attribute, spec.Kind)
}

func toSeedItems(value any, resultType ResultType, keyAttribute string, fields map[string]provider.Type) ([]map[string]types.AttributeValue, error) {
	var items []map[string]types.AttributeValue
	if resultType == ResultTypeItemMap {
		records, err := toSeedContainer[map[string]map[string]any](value)
		if err != nil {
			return nil, err
		}
		for key, row := range records {
			item, err := toSeedItem(row, fields)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			if key == "" {
				return nil, fmt.Errorf("empty DynamoDB key")
			}
			item[keyAttribute] = &types.AttributeValueMemberS{Value: key}
			items = append(items, item)
		}
		return items, nil
	}
	rows, err := toSeedContainer[[]map[string]any](value)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(rows))
	for i, row := range rows {
		item, err := toSeedItem(row, fields)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i, err)
		}
		key := item[keyAttribute].(*types.AttributeValueMemberS).Value
		if key == "" || seen[key] {
			return nil, fmt.Errorf("empty or duplicate DynamoDB key %q", key)
		}
		seen[key] = true
		items = append(items, item)
	}
	return items, nil
}

func toSeedItem(row map[string]any, fields map[string]provider.Type) (map[string]types.AttributeValue, error) {
	if len(row) != len(fields) {
		return nil, fmt.Errorf("row fields must match declared fields")
	}
	item := make(map[string]types.AttributeValue, len(fields))
	for name, spec := range fields {
		value, exists := row[name]
		if !exists {
			return nil, fmt.Errorf("missing field %q", name)
		}
		attribute, err := toSeedAttribute(value, spec)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", name, err)
		}
		item[name] = attribute
	}
	return item, nil
}

func toSeedAttribute(value any, spec provider.Type) (types.AttributeValue, error) {
	raw := reflect.ValueOf(value)
	// Generated nullable fields arrive as pointers; map values may use nil directly.
	for raw.IsValid() && (raw.Kind() == reflect.Pointer || raw.Kind() == reflect.Interface) {
		if raw.IsNil() {
			raw = reflect.Value{}
			break
		}
		raw = raw.Elem()
	}
	if !raw.IsValid() {
		if !spec.Nullable {
			return nil, fmt.Errorf("null is not allowed")
		}
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	switch spec.Kind {
	case provider.KindString:
		text, ok := raw.Interface().(string)
		if !ok {
			return nil, fmt.Errorf("got %T, want string", raw.Interface())
		}
		return &types.AttributeValueMemberS{Value: text}, nil
	case provider.KindInt64:
		number, ok := raw.Interface().(int64)
		if !ok {
			return nil, fmt.Errorf("got %T, want int64", raw.Interface())
		}
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(number, 10)}, nil
	case provider.KindBool:
		boolean, ok := raw.Interface().(bool)
		if !ok {
			return nil, fmt.Errorf("got %T, want bool", raw.Interface())
		}
		return &types.AttributeValueMemberBOOL{Value: boolean}, nil
	default:
		return nil, fmt.Errorf("unsupported model type %s", spec.Kind)
	}
}

// Decode containers without coercing their scalar values; attributes are checked
// against the declared types separately, including nullable interface values.
func toSeedContainer[T any](value any) (T, error) {
	var result T
	raw := reflect.ValueOf(value)
	if !raw.IsValid() || raw.Kind() != reflect.TypeFor[T]().Kind() || raw.IsNil() {
		return result, fmt.Errorf("expected non-null %s", reflect.TypeFor[T]().Kind())
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &result, TagName: "json"})
	if err != nil {
		return result, err
	}
	if err := decoder.Decode(value); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}
