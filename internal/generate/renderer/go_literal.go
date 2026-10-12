package renderer

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// goLiteral keeps YAML configuration values as Go values in generated code.
func goLiteral(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "nil", nil
	case string:
		return strconv.Quote(value), nil
	case bool:
		return strconv.FormatBool(value), nil
	case int:
		return strconv.Itoa(value), nil
	case int64:
		return "int64(" + strconv.FormatInt(value, 10) + ")", nil
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	case []any:
		return listLiteral(value)
	case map[string]any:
		return mapLiteral(value)
	default:
		return "", fmt.Errorf("unsupported YAML value %T", value)
	}
}

func listLiteral(values []any) (string, error) {
	items := make([]string, 0, len(values))
	for _, value := range values {
		literal, err := goLiteral(value)
		if err != nil {
			return "", err
		}
		items = append(items, literal)
	}
	return compositeLiteral("[]any", items), nil
}

func mapLiteral(values map[string]any) (string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		literal, err := goLiteral(values[key])
		if err != nil {
			return "", fmt.Errorf("YAML key %q: %w", key, err)
		}
		items = append(items, strconv.Quote(key)+": "+literal)
	}
	return compositeLiteral("map[string]any", items), nil
}

// compositeLiteral separates nonempty members before gofmt adds indentation.
func compositeLiteral(typeName string, members []string) string {
	if len(members) == 0 {
		return typeName + "{}"
	}
	return typeName + "{\n" + strings.Join(members, ",\n") + ",\n}"
}
