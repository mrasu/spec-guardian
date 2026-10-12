package gcmp

import (
	"fmt"
	"reflect"
)

// ObservedList records list elements in their observed order.
type ObservedList[T any] struct {
	configured bool
	values     []T
}

// ListObserved records a list without changing its order or elements.
func ListObserved[T any](values []T) *ObservedList[T] {
	return newObservedList(values)
}

func newObservedList[T any](values []T) *ObservedList[T] {
	return &ObservedList[T]{
		configured: true,
		values:     values,
	}
}

// Validate checks that the list and all its elements were configured.
func (l *ObservedList[T]) Validate(path string) error {
	if l == nil || !l.configured {
		return fmt.Errorf("%s: list observation is not configured", path)
	}
	return validateContainerValue(l.containerValue(), path)
}

// DiagnosticValue returns the elements or an unset explanation.
func (l *ObservedList[T]) DiagnosticValue() any {
	if l == nil || !l.configured {
		return []string{"list observation is not configured"}
	}
	return comparisonDiagnosticValue(l.containerValue())
}

func (l *ObservedList[T]) containerKind() containerKind  { return containerList }
func (l *ObservedList[T]) containerValue() reflect.Value { return reflect.ValueOf(l.values) }
