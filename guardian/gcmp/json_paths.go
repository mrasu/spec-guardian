package gcmp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

func matchJSONPaths(path string, got, want any, ignoredPaths, unorderedPaths []string) error {
	normalizedGot, err := normalizeJSONPaths(got, ignoredPaths, unorderedPaths)
	if err != nil {
		return fmt.Errorf("%s: normalize observed value: %w", path, err)
	}
	normalizedWant, err := normalizeJSONPaths(want, ignoredPaths, unorderedPaths)
	if err != nil {
		return fmt.Errorf("%s: normalize allowed value: %w", path, err)
	}
	if reflect.DeepEqual(normalizedGot, normalizedWant) {
		return nil
	}
	return comparisonMismatchError(path, normalizedGot, normalizedWant)
}

func normalizeJSONPaths(value any, ignoredPaths, unorderedPaths []string) (any, error) {
	normalized, err := withoutJSONPaths(value, ignoredPaths)
	if err != nil {
		return nil, err
	}
	// Normalize inner lists before using their contents to order outer lists.
	paths := slices.Clone(unorderedPaths)
	slices.SortStableFunc(paths, func(a, b string) int {
		return len(b) - len(a)
	})
	for _, path := range paths {
		var segments []string
		if path != "" {
			segments = strings.Split(path, "/")
		}
		if err := sortJSONLists(normalized, segments); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func sortJSONLists(value any, segments []string) error {
	if len(segments) == 0 {
		return sortJSONList(value)
	}
	if value == nil {
		return nil
	}
	switch segments[0] {
	case "[]":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("[] requires a list")
		}
		for _, item := range items {
			if err := sortJSONLists(item, segments[1:]); err != nil {
				return err
			}
		}
	case "{}":
		items, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("{} requires a map")
		}
		for _, item := range items {
			if err := sortJSONLists(item, segments[1:]); err != nil {
				return err
			}
		}
	default:
		fields, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("field %q requires a message", segments[0])
		}
		if child, found := fields[segments[0]]; found {
			return sortJSONLists(child, segments[1:])
		}
	}
	return nil
}

func sortJSONList(value any) error {
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("expected a list, got %T", value)
	}
	type element struct {
		value any
		key   string
	}
	elements := make([]element, len(items))
	for i, item := range items {
		key, err := json.Marshal(item)
		if err != nil {
			return err
		}
		elements[i] = element{value: item, key: string(key)}
	}
	slices.SortStableFunc(elements, func(a, b element) int {
		return strings.Compare(a.key, b.key)
	})
	for i, element := range elements {
		items[i] = element.value
	}
	return nil
}

func withoutJSONPaths(value any, paths []string) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	for _, path := range paths {
		decoded = removeJSONPath(decoded, strings.Split(path, "/"))
	}
	return decoded, nil
}

func removeJSONPath(value any, segments []string) any {
	if len(segments) == 0 || segments[0] == "" {
		return nil
	}
	switch segments[0] {
	case "[]":
		items, ok := value.([]any)
		if !ok {
			return value
		}
		for index := range items {
			items[index] = removeJSONPath(items[index], segments[1:])
		}
	case "{}":
		mapping, ok := value.(map[string]any)
		if !ok {
			return value
		}
		for key := range mapping {
			mapping[key] = removeJSONPath(mapping[key], segments[1:])
		}
	default:
		mapping, ok := value.(map[string]any)
		if !ok {
			return value
		}
		if len(segments) == 1 {
			delete(mapping, segments[0])
			return mapping
		}
		if child, found := mapping[segments[0]]; found {
			mapping[segments[0]] = removeJSONPath(child, segments[1:])
		}
	}
	return value
}
