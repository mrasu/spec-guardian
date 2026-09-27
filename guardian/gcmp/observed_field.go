package gcmp

import (
	"fmt"
	"reflect"
)

type observedFieldMode uint8

const (
	observedFieldUnset observedFieldMode = iota
	observedFieldValue
	observedFieldUnobservable
)

// ObservedField contains an observed value or the reason it could not be observed.
type ObservedField[T any] struct {
	mode   observedFieldMode
	value  T
	reason string
}

// NewObservedField creates an unset observed field.
func NewObservedField[T any]() *ObservedField[T] {
	return &ObservedField[T]{}
}

// Observed records an observed value.
func Observed[T any](value T) *ObservedField[T] {
	field := NewObservedField[T]()
	field.mode = observedFieldValue
	field.value = value
	return field
}

// Unobservable records why a field could not be observed.
func Unobservable[T any](reason string) *ObservedField[T] {
	field := NewObservedField[T]()
	field.mode = observedFieldUnobservable
	field.reason = reason
	return field
}

// Validate reports whether the observed field is completely configured.
func (f *ObservedField[T]) Validate(path string) error {
	if f == nil {
		return fmt.Errorf("%s: neither Observed nor Unobservable was configured", path)
	}
	switch f.mode {
	case observedFieldValue:
		return nil
	case observedFieldUnobservable:
		if f.reason == "" {
			return fmt.Errorf("%s: Unobservable requires a reason", path)
		}
		return nil
	default:
		return fmt.Errorf("%s: neither Observed nor Unobservable was configured", path)
	}
}

// MatchComparison checks this observed field against an allowed field comparison.
func (f *ObservedField[T]) MatchComparison(path string, allowed *ComparisonField[T]) error {
	if err := f.Validate(path); err != nil {
		return fmt.Errorf("observed field: %w", err)
	}
	if err := allowed.Validate(path); err != nil {
		return fmt.Errorf("allowed comparison: %w", err)
	}
	if f.mode == observedFieldUnobservable || allowed.mode == comparisonFieldIgnore {
		return nil
	}
	if allowed.mode != comparisonFieldEqual {
		return fmt.Errorf("%s: observed-field matching requires Equal or Ignore", path)
	}
	if reflect.DeepEqual(f.value, allowed.value) {
		return nil
	}
	return comparisonMismatchError(path, f.value, allowed.value)
}

// DiagnosticValue returns the observed value or why it was unavailable.
func (f *ObservedField[T]) DiagnosticValue() any {
	if f == nil {
		return []string{"observed field is not configured"}
	}
	if f.mode == observedFieldUnobservable {
		return []string{"[UNOBSERVABLE; " + f.reason + "]"}
	}
	if f.mode == observedFieldValue {
		return f.value
	}
	return []string{"observed field is not configured"}
}
