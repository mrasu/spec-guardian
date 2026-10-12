package field

import "fmt"

func toString(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case []byte:
		return string(value), nil
	default:
		return "", fmt.Errorf("got %T, want SQL text", value)
	}
}
