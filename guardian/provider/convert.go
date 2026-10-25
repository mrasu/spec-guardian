package provider

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

// Convert checks the value returned by Observer.Observe and converts it to T.
// Struct field names come from their json tags. Missing fields, extra fields,
// and incompatible values are errors. Pointers represent nullable values,
// including maps and lists. A non-nil pointer to a nil collection is invalid;
// empty collections remain valid.
//
// For a single message, a provider can return a map whose keys match the
// generated struct's field names:
//
//	type State struct {
//		Status string `json:"status"`
//	}
//	value := map[string]any{"status": "ready"}
//	state, err := Convert[State](value)
//	// state == State{Status: "ready"}
//
// For a map of messages, each map value follows the same field shape. A null
// field becomes a nil pointer:
//
//	type Record struct {
//		Name string  `json:"name"`
//		Note *string `json:"note"`
//	}
//	value := map[string]any{
//		"one": map[string]any{"name": "order", "note": nil},
//	}
//	records, err := Convert[map[string]Record](value)
//	// records == map[string]Record{"one": {Name: "order"}}
func Convert[T any](value any) (T, error) {
	var result T
	target := reflect.TypeFor[T]()
	if err := validateObservedValue(value, target); err != nil {
		return result, err
	}
	if isNull(value) {
		return result, nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      &result,
		TagName:     "json",
		MatchName:   func(mapKey, fieldName string) bool { return mapKey == fieldName },
		ErrorUnused: true,
		ErrorUnset:  true,
	})
	if err != nil {
		return result, err
	}
	if err := decoder.Decode(value); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

func validateObservedValue(value any, target reflect.Type) error {
	if isNull(value) {
		if target.Kind() == reflect.Pointer && (isPrimitive(target.Elem()) || target.Elem().Kind() == reflect.Struct || target.Elem().Kind() == reflect.Map || target.Elem().Kind() == reflect.Slice) {
			return nil
		}
		if target.Kind() == reflect.Map || target.Kind() == reflect.Pointer && target.Elem().Kind() == reflect.Map {
			return fmt.Errorf("null map is not allowed")
		}
		return fmt.Errorf("null is not allowed")
	}
	if target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	raw := reflect.ValueOf(value)
	if raw.Kind() == reflect.Pointer {
		return validateObservedValue(raw.Elem().Interface(), target)
	}
	if isBytes(target) {
		if _, ok := value.([]byte); !ok {
			return fmt.Errorf("got %T, want []byte", value)
		}
		return nil
	}
	switch target.Kind() {
	case reflect.Int64:
		if _, ok := value.(int64); !ok {
			return fmt.Errorf("got %T, want int64", value)
		}
	case reflect.Float64:
		number, ok := value.(float64)
		if !ok {
			return fmt.Errorf("got %T, want float64", value)
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("double must be finite")
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("got %T, want bool", value)
		}
	case reflect.Map:
		if raw.Kind() != reflect.Map {
			return nil
		}
		for iter := raw.MapRange(); iter.Next(); {
			if err := validateObservedValue(iter.Value().Interface(), target.Elem()); err != nil {
				return fmt.Errorf("map key %q: %w", iter.Key(), err)
			}
		}
	case reflect.Slice, reflect.Array:
		if raw.Kind() != reflect.Slice && raw.Kind() != reflect.Array {
			return nil
		}
		for index := 0; index < raw.Len(); index++ {
			if err := validateObservedValue(raw.Index(index).Interface(), target.Elem()); err != nil {
				return fmt.Errorf("list index %d: %w", index, err)
			}
		}
	case reflect.Struct:
		for field := range target.Fields() {
			name := jsonFieldName(field)
			if !field.IsExported() || name == "-" {
				continue
			}
			item := messageFieldValue(raw, name)
			if item.IsValid() && item.CanInterface() {
				if err := validateObservedValue(item.Interface(), field.Type); err != nil {
					return fmt.Errorf("message field %q: %w", name, err)
				}
			}
		}
	}
	return nil
}

func messageFieldValue(raw reflect.Value, name string) reflect.Value {
	switch raw.Kind() {
	case reflect.Map:
		if raw.Type().Key().Kind() == reflect.String {
			return raw.MapIndex(reflect.ValueOf(name).Convert(raw.Type().Key()))
		}
	case reflect.Struct:
		for field := range raw.Type().Fields() {
			if field.IsExported() && jsonFieldName(field) == name {
				return raw.FieldByIndex(field.Index)
			}
		}
	}
	return reflect.Value{}
}

func isBytes(target reflect.Type) bool {
	return target.Kind() == reflect.Slice && target.Elem() == reflect.TypeFor[byte]()
}

func jsonFieldName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" {
		return field.Name
	}
	return name
}

func isPrimitive(target reflect.Type) bool {
	switch target.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return true
	default:
		return false
	}
}

func isNull(value any) bool {
	if value == nil {
		return true
	}
	raw := reflect.ValueOf(value)
	return (raw.Kind() == reflect.Pointer || raw.Kind() == reflect.Map || raw.Kind() == reflect.Slice) && raw.IsNil()
}
