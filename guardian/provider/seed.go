package provider

import (
	"fmt"
	"reflect"
	"sort"
)

// ValidateSeedFields checks that the result preserves the request's field order.
func ValidateSeedFields(request SeedRequest, result SeedResult) error {
	if len(request.Values) != len(result.Values) {
		return fmt.Errorf("seed returned %d fields, want %d", len(result.Values), len(request.Values))
	}
	for i, expected := range request.Values {
		if result.Values[i].Field != expected.Field {
			return fmt.Errorf("seed field %d: got %q, want %q", i, result.Values[i].Field, expected.Field)
		}
	}
	return nil
}

// ValidateSeedValue checks shape and position-preserving equality with a model
// input, reporting mismatches relative to field. Collection comparison settings
// do not relax the seed contract.
func ValidateSeedValue[T any](field string, expected T, actual any) error {
	converted, err := Convert[T](actual)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return compareSeedValues(reflect.ValueOf(expected), reflect.ValueOf(converted), field)
}

func compareSeedValues(expected, actual reflect.Value, path string) error {
	if !expected.IsValid() || !actual.IsValid() {
		if expected.IsValid() == actual.IsValid() {
			return nil
		}
		return fmt.Errorf("%s: seed value differs from input: null mismatch", path)
	}
	if reflect.DeepEqual(expected.Interface(), actual.Interface()) {
		return nil
	}
	if expected.Type() != actual.Type() {
		return fmt.Errorf("%s: seed value type is %s, want %s", path, actual.Type(), expected.Type())
	}
	switch expected.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !expected.IsNil() && !actual.IsNil() {
			return compareSeedValues(expected.Elem(), actual.Elem(), path)
		}
	case reflect.Struct:
		return compareSeedMessage(expected, actual, path)
	case reflect.Slice, reflect.Array:
		if expected.Kind() != reflect.Slice || (!expected.IsNil() && !actual.IsNil()) {
			return compareSeedList(expected, actual, path)
		}
	case reflect.Map:
		if !expected.IsNil() && !actual.IsNil() {
			return compareSeedMap(expected, actual, path)
		}
	}
	return fmt.Errorf("%s: seed value differs from input: got %#v, want %#v", path, actual.Interface(), expected.Interface())
}

func compareSeedMessage(expected, actual reflect.Value, path string) error {
	for field := range expected.Type().Fields() {
		if !field.IsExported() || jsonFieldName(field) == "-" {
			continue
		}
		if err := compareSeedValues(
			expected.FieldByIndex(field.Index),
			actual.FieldByIndex(field.Index),
			path+"."+jsonFieldName(field),
		); err != nil {
			return err
		}
	}
	return nil
}

func compareSeedList(expected, actual reflect.Value, path string) error {
	if expected.Len() != actual.Len() {
		return fmt.Errorf("%s: seed list length is %d, want %d", path, actual.Len(), expected.Len())
	}
	for i := 0; i < expected.Len(); i++ {
		if err := compareSeedValues(expected.Index(i), actual.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func compareSeedMap(expected, actual reflect.Value, path string) error {
	keys := expected.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, key := range keys {
		itemPath := fmt.Sprintf("%s[%q]", path, key.String())
		value := actual.MapIndex(key)
		if !value.IsValid() {
			return fmt.Errorf("%s: seed map key is missing", itemPath)
		}
		if err := compareSeedValues(expected.MapIndex(key), value, itemPath); err != nil {
			return err
		}
	}
	extra := actual.MapKeys()
	sort.Slice(extra, func(i, j int) bool { return extra[i].String() < extra[j].String() })
	for _, key := range extra {
		if !expected.MapIndex(key).IsValid() {
			return fmt.Errorf("%s[%q]: unexpected seed map key", path, key.String())
		}
	}
	return nil
}
