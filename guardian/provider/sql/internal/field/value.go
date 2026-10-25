package field

import (
	"fmt"
	"math"
	"reflect"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

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

func toScalar(value any, spec provider.Type) (any, error) {
	if value == nil {
		if spec.Nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected SQL null")
	}
	switch spec.Kind {
	case provider.KindString:
		return toString(value)
	case provider.KindInt64:
		if v, ok := value.(int64); ok {
			return v, nil
		}
		// MySQL returns BIGINT UNSIGNED as uint64. Preserve its value only
		// when the declared model type can represent it exactly.
		if v, ok := value.(uint64); ok {
			if v > math.MaxInt64 {
				return nil, fmt.Errorf("SQL integer %d overflows int64", v)
			}
			return int64(v), nil
		}
	case provider.KindBool:
		if v, ok := value.(bool); ok {
			return v, nil
		}
		if v, ok := value.(int64); ok && (v == 0 || v == 1) {
			return v == 1, nil
		}
	case provider.KindDouble:
		if v, ok := value.(float64); ok {
			return v, nil
		}
	case provider.KindBytes:
		if v, ok := value.([]byte); ok {
			return append([]byte{}, v...), nil
		}
	}
	return nil, fmt.Errorf("got SQL value %T, want %s", value, spec.Kind)
}

func toSeedValue(value any, spec provider.Type) (any, error) {
	raw := reflect.ValueOf(value)
	if raw.IsValid() && raw.Kind() == reflect.Pointer {
		if raw.IsNil() {
			value = nil
		} else {
			value = raw.Elem().Interface()
		}
	}
	if value == nil {
		if spec.Nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("null is not allowed")
	}
	switch spec.Kind {
	case provider.KindString:
		return toSeedScalar[string](value)
	case provider.KindInt64:
		return toSeedScalar[int64](value)
	case provider.KindBool:
		return toSeedScalar[bool](value)
	case provider.KindDouble:
		return toSeedScalar[float64](value)
	case provider.KindBytes:
		return toSeedScalar[[]byte](value)
	default:
		return nil, fmt.Errorf("unsupported seed type %s", spec.Kind)
	}
}

func toSeedScalar[T any](value any) (any, error) {
	result, err := provider.Convert[T](value)
	if err != nil {
		return nil, err
	}
	return result, nil
}
