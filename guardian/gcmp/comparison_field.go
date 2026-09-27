package gcmp

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
)

type comparisonFieldMode uint8

const (
	comparisonFieldUnset comparisonFieldMode = iota
	comparisonFieldEqual
	comparisonFieldIgnore
	comparisonFieldCustom
	comparisonFieldCandidate
)

// ComparisonField configures how one expected FizzBee field is checked.
type ComparisonField[T any] struct {
	mode             comparisonFieldMode
	value            T
	actual           any
	reason           string
	compare          func(T) (ok bool, reason string)
	compareCandidate func(T, any) (ok bool, reason string)
}

// FieldComparison is the legacy name for ComparisonField.
type FieldComparison[T any] = ComparisonField[T]

// NewComparisonField creates an unset field comparison.
func NewComparisonField[T any]() *ComparisonField[T] {
	return &ComparisonField[T]{}
}

// Equal compares an observed value with the expected value using DeepEqual.
func Equal[T any](value T) *ComparisonField[T] {
	comparison := NewComparisonField[T]()
	comparison.mode = comparisonFieldEqual
	comparison.value = value
	return comparison
}

// Ignore excludes a field from comparison and records the reason.
func Ignore[T any](reason string) *ComparisonField[T] {
	comparison := NewComparisonField[T]()
	comparison.mode = comparisonFieldIgnore
	comparison.reason = reason
	return comparison
}

// Compare checks an expected value with a custom function. Actual is retained
// solely for conformance-test diagnostics.
func Compare[T any](actual any, compare func(T) (bool, string)) *ComparisonField[T] {
	comparison := NewComparisonField[T]()
	comparison.mode = comparisonFieldCustom
	comparison.actual = actual
	comparison.compare = compare
	return comparison
}

// CompareWithCandidate checks a field using its expected value and the allowed
// state currently being matched. Actual is retained for diagnostics.
func CompareWithCandidate[T, Candidate any](actual any, compare func(T, Candidate) (bool, string)) *ComparisonField[T] {
	comparison := NewComparisonField[T]()
	comparison.mode = comparisonFieldCandidate
	comparison.actual = actual
	if compare != nil {
		comparison.compareCandidate = func(want T, candidate any) (bool, string) {
			state, ok := candidate.(Candidate)
			if !ok {
				return false, fmt.Sprintf("candidate has type %T, want %T", candidate, *new(Candidate))
			}
			return compare(want, state)
		}
	}
	return comparison
}

// DiagnosticValue returns the observed value or an explanation suitable for a
// conformance-test failure report.
func (c *ComparisonField[T]) DiagnosticValue() any {
	if c == nil {
		return []string{"comparison is not configured"}
	}
	switch c.mode {
	case comparisonFieldEqual:
		return c.value
	case comparisonFieldIgnore:
		return []string{"[IGNORED; " + c.reason + "]"}
	case comparisonFieldCustom, comparisonFieldCandidate:
		return c.actual
	default:
		return []string{"comparison is not configured"}
	}
}

// Validate reports whether a comparison has a complete configuration.
func (c *ComparisonField[T]) Validate(path string) error {
	if c == nil {
		return fmt.Errorf("%s: neither Equal, Ignore, nor Compare was configured", path)
	}
	switch c.mode {
	case comparisonFieldEqual:
		return nil
	case comparisonFieldIgnore:
		if c.reason == "" {
			return fmt.Errorf("%s: Ignore requires a reason", path)
		}
		return nil
	case comparisonFieldCustom:
		if c.compare == nil {
			return fmt.Errorf("%s: Compare requires a function", path)
		}
		return nil
	case comparisonFieldCandidate:
		if c.compareCandidate == nil {
			return fmt.Errorf("%s: CompareWithCandidate requires a function", path)
		}
		return nil
	default:
		return fmt.Errorf("%s: neither Equal, Ignore, nor Compare was configured", path)
	}
}

// Match reports whether the expected value satisfies this comparison.
func (c *ComparisonField[T]) Match(path string, want T) error {
	return c.MatchWithCandidate(path, want, nil)
}

// MatchWithCandidate checks the expected field against this comparison with
// access to the allowed state currently being matched.
func (c *ComparisonField[T]) MatchWithCandidate(path string, want T, candidate any) error {
	if err := c.Validate(path); err != nil {
		return err
	}
	switch c.mode {
	case comparisonFieldEqual:
		if reflect.DeepEqual(c.value, want) {
			return nil
		}
		return comparisonMismatchError(path, c.value, want)
	case comparisonFieldIgnore:
		return nil
	case comparisonFieldCustom:
		if ok, reason := c.compare(want); !ok {
			return fmt.Errorf("%s: %s", path, reason)
		}
		return nil
	case comparisonFieldCandidate:
		if ok, reason := c.compareCandidate(want, candidate); !ok {
			return fmt.Errorf("%s: %s", path, reason)
		}
		return nil
	default:
		return c.Validate(path)
	}
}

func comparisonMismatchError(path string, got, want any) error {
	diff, ok := comparisonDiff(want, got)
	if !ok {
		return fmt.Errorf("%s: got %s, want %s", path, formatComparisonValue(got), formatComparisonValue(want))
	}
	return fmt.Errorf("%s (-want +got):\n%s", path, strings.TrimSuffix(diff, "\n"))
}

func comparisonDiff(want, got any) (diff string, ok bool) {
	defer func() {
		if recover() != nil {
			diff = ""
			ok = false
		}
	}()
	return cmp.Diff(want, got), true
}

func formatComparisonValue(value any) string {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Pointer {
		return fmt.Sprintf("%#v", value)
	}
	if reflected.IsNil() {
		return fmt.Sprintf("(%T)(nil)", value)
	}
	return fmt.Sprintf("(%T)(%#v)", value, reflected.Elem().Interface())
}
