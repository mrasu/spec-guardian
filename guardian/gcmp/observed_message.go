package gcmp

import (
	"fmt"
	"reflect"
)

// ObservedMessage records a message, including an explicitly observed null.
type ObservedMessage[T any] struct {
	configured bool
	value      *T
}

// MessageObserved records a message. A nil value records null, not an unset observation.
func MessageObserved[T any](value *T) *ObservedMessage[T] {
	return newObservedMessage(value)
}

func newObservedMessage[T any](value *T) *ObservedMessage[T] {
	requireMessageType[T]()
	return &ObservedMessage[T]{
		configured: true,
		value:      value,
	}
}

// Validate checks that the message and its non-null fields were configured.
func (m *ObservedMessage[T]) Validate(path string) error {
	if m == nil || !m.configured {
		return fmt.Errorf("%s: message observation is not configured", path)
	}
	return validateContainerValue(m.containerValue(), path)
}

// DiagnosticValue returns the message's field values or an unset explanation.
func (m *ObservedMessage[T]) DiagnosticValue() any {
	if m == nil || !m.configured {
		return []string{"message observation is not configured"}
	}
	return comparisonDiagnosticValue(m.containerValue())
}

func (m *ObservedMessage[T]) containerKind() containerKind  { return containerMessage }
func (m *ObservedMessage[T]) containerValue() reflect.Value { return reflect.ValueOf(m.value) }

func requireMessageType[T any]() {
	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		panic("message requires a struct type")
	}
}
