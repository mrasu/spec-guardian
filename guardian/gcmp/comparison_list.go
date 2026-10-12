package gcmp

import (
	"fmt"
	"reflect"
)

// ComparisonList configures element comparisons, ordering, or a whole-list ignore.
type ComparisonList[T any] struct {
	configured    bool
	ignored       bool
	ignoredReason string
	values        []T
	ignoreOrder   bool
}

// ListEqual compares elements in their declared order.
func ListEqual[T any](values []T) *ComparisonList[T] {
	return newComparisonList(values, false, false, "")
}

// ListUnordered compares elements one-to-one without requiring their order to match.
func ListUnordered[T any](values []T) *ComparisonList[T] {
	return newComparisonList(values, true, false, "")
}

// ListIgnore ignores the entire list and records why.
func ListIgnore[T any](ignoredReason string) *ComparisonList[T] {
	return newComparisonList[T](nil, false, true, ignoredReason)
}

func newComparisonList[T any](values []T, ignoreOrder, ignored bool, ignoredReason string) *ComparisonList[T] {
	return &ComparisonList[T]{
		configured:    true,
		ignored:       ignored,
		ignoredReason: ignoredReason,
		values:        values,
		ignoreOrder:   ignoreOrder,
	}
}

// Validate checks the comparison, its ignore reason, and its elements.
func (l *ComparisonList[T]) Validate(path string) error {
	if l == nil || !l.configured {
		return fmt.Errorf("%s: list comparison is not configured", path)
	}
	if l.ignored {
		return validateIgnoredReason(path, l.ignoredReason)
	}
	return validateContainerValue(l.containerValue(), path)
}

// DiagnosticValue returns the expected elements or the ignore reason.
func (l *ComparisonList[T]) DiagnosticValue() any {
	if l == nil || !l.configured {
		return []string{"list comparison is not configured"}
	}
	if l.ignored {
		return []string{"[IGNORED; " + l.ignoredReason + "]"}
	}
	return comparisonDiagnosticValue(l.containerValue())
}

func (l *ComparisonList[T]) containerKind() containerKind      { return containerList }
func (l *ComparisonList[T]) containerValue() reflect.Value     { return reflect.ValueOf(l.values) }
func (l *ComparisonList[T]) containerComparison() (bool, bool) { return l.ignored, l.ignoreOrder }
