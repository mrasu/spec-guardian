package gcmp

import (
	"fmt"
	"reflect"
)

// ComparisonMessage compares a message's fields or ignores the whole message.
type ComparisonMessage[T any] struct {
	configured    bool
	ignored       bool
	ignoredReason string
	value         *T
}

// MessageEqual compares a message recursively. A nil value expects null.
func MessageEqual[T any](value *T) *ComparisonMessage[T] {
	return newComparisonMessage(value, false, "")
}

// MessageIgnore ignores the entire message and records why.
func MessageIgnore[T any](ignoredReason string) *ComparisonMessage[T] {
	return newComparisonMessage[T](nil, true, ignoredReason)
}

func newComparisonMessage[T any](value *T, ignored bool, ignoredReason string) *ComparisonMessage[T] {
	requireMessageType[T]()
	return &ComparisonMessage[T]{
		configured:    true,
		ignored:       ignored,
		ignoredReason: ignoredReason,
		value:         value,
	}
}

// Validate checks the comparison, its ignore reason, and its non-null fields.
func (m *ComparisonMessage[T]) Validate(path string) error {
	if m == nil || !m.configured {
		return fmt.Errorf("%s: message comparison is not configured", path)
	}
	if m.ignored {
		return validateIgnoredReason(path, m.ignoredReason)
	}
	return validateContainerValue(m.containerValue(), path)
}

// DiagnosticValue returns the expected fields or the ignore reason.
func (m *ComparisonMessage[T]) DiagnosticValue() any {
	if m == nil || !m.configured {
		return []string{"message comparison is not configured"}
	}
	if m.ignored {
		return []string{"[IGNORED; " + m.ignoredReason + "]"}
	}
	return comparisonDiagnosticValue(m.containerValue())
}

func (m *ComparisonMessage[T]) containerKind() containerKind      { return containerMessage }
func (m *ComparisonMessage[T]) containerValue() reflect.Value     { return reflect.ValueOf(m.value) }
func (m *ComparisonMessage[T]) containerComparison() (bool, bool) { return m.ignored, false }
