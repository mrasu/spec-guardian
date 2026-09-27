package gcmp

import (
	"fmt"
	"reflect"
)

// StateFor binds a comparison struct to the expected state type.
type StateFor[Expected any] interface {
	ExpectedState() Expected
}

// StateComparator applies field comparisons throughout a state struct.
// The state method binds the observed and expected types at compile time.
type StateComparator[Expected any, Actual StateFor[Expected]] struct {
	actual Actual
}

// ObservedStateComparator compares an observed state with an allowed comparison state.
type ObservedStateComparator[Allowed, Observed any] struct {
	observed Observed
}

// NewStateComparator constructs a comparator for a pointer to a state struct.
func NewStateComparator[Expected any, Actual StateFor[Expected]](actual Actual) *StateComparator[Expected, Actual] {
	actualValue := reflect.ValueOf(actual)
	if !actualValue.IsValid() || actualValue.Kind() != reflect.Pointer || actualValue.IsNil() {
		panic("comparison requires an actual state")
	}
	if reflect.TypeFor[Expected]().Kind() != reflect.Struct || actualValue.Elem().Kind() != reflect.Struct {
		panic("comparison requires struct states")
	}
	return &StateComparator[Expected, Actual]{actual: actual}
}

// NewObservedStateComparator constructs a comparator for an observed state and allowed comparison state.
func NewObservedStateComparator[Allowed, Observed any](observed Observed) *ObservedStateComparator[Allowed, Observed] {
	if _, err := comparisonStructValue(observed); err != nil {
		panic(err)
	}
	return &ObservedStateComparator[Allowed, Observed]{observed: observed}
}

// Validate reports the first unconfigured observed field.
func (c *ObservedStateComparator[Allowed, Observed]) Validate() error {
	if c == nil {
		return fmt.Errorf("comparison is nil")
	}
	observed, err := comparisonStructValue(c.observed)
	if err != nil {
		return err
	}
	return validateComparisonFields(observed)
}

// Match returns differences from one allowed comparison state.
func (c *ObservedStateComparator[Allowed, Observed]) Match(allowed Allowed) []string {
	if c == nil {
		return []string{"comparison is nil"}
	}
	allowedValue, err := comparisonStructValue(allowed)
	if err != nil {
		return []string{"allowed comparison: " + err.Error()}
	}
	if err := validateComparisonFields(allowedValue); err != nil {
		return []string{"allowed comparison: " + err.Error()}
	}

	observedValue := reflect.ValueOf(c.observed).Elem()
	return walkComparisonFieldsAll(observedValue, allowedValue, "", matchComparisonField)
}

// DiagnosticValue returns the observed fields for a mismatch report.
func (c *ObservedStateComparator[Allowed, Observed]) DiagnosticValue() any {
	if c == nil {
		return nil
	}
	return comparisonDiagnosticValue(reflect.ValueOf(c.observed).Elem())
}

func comparisonStructValue[State any](state State) (reflect.Value, error) {
	stateValue := reflect.ValueOf(state)
	if !stateValue.IsValid() || stateValue.Kind() != reflect.Pointer || stateValue.IsNil() {
		return reflect.Value{}, fmt.Errorf("comparison requires a non-nil pointer")
	}
	if stateValue.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("comparison requires a pointer to a struct")
	}
	return stateValue.Elem(), nil
}

func validateComparisonFields(state reflect.Value) error {
	return walkComparisonFields(state, state, "", func(actual, _ reflect.Value, path string) error {
		validator, ok := actual.Interface().(interface{ Validate(string) error })
		if !ok {
			return fmt.Errorf("%s: field is not a comparison", path)
		}
		return validator.Validate(path)
	})
}

func matchComparisonField(actual, allowed reflect.Value, path string) error {
	method := actual.MethodByName("MatchComparison")
	if !method.IsValid() || method.Type().NumIn() != 2 || method.Type().NumOut() != 1 || !allowed.Type().AssignableTo(method.Type().In(1)) {
		return fmt.Errorf("%s: comparison fields have incompatible types", path)
	}
	result := method.Call([]reflect.Value{reflect.ValueOf(path), allowed})[0]
	if !result.IsNil() {
		return result.Interface().(error)
	}
	return nil
}

// Validate reports the first unconfigured field comparison.
func (c *StateComparator[Expected, Actual]) Validate() error {
	return c.walk(*new(Expected), func(actual, _ reflect.Value, path string) error {
		validator, ok := actual.Interface().(interface{ Validate(string) error })
		if !ok {
			return fmt.Errorf("%s: field is not a comparison", path)
		}
		return validator.Validate(path)
	})
}

// Match returns differences from all fields of the expected state.
func (c *StateComparator[Expected, Actual]) Match(want Expected) []string {
	candidate := reflect.ValueOf(want)
	if c == nil {
		return []string{"comparison is nil"}
	}
	actualValue := reflect.ValueOf(c.actual).Elem()
	wantValue := reflect.ValueOf(want)
	return walkComparisonFieldsAll(actualValue, wantValue, "", func(actual, expected reflect.Value, path string) error {
		return matchField(actual, expected, path, candidate)
	})
}

func walkComparisonFieldsAll(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	if !actual.IsValid() || !want.IsValid() {
		return []string{fmt.Sprintf("%s: field is missing from comparison or expected state", path)}
	}
	if actual.Kind() != reflect.Struct {
		if err := action(actual, want, path); err != nil {
			return []string{err.Error()}
		}
		return nil
	}
	if want.Kind() != reflect.Struct {
		return []string{fmt.Sprintf("%s: expected state field is not a struct", path)}
	}

	var diffs []string
	for index := range actual.NumField() {
		name := actual.Type().Field(index).Name
		fieldPath := name
		if path != "" {
			fieldPath = path + "." + name
		}
		diffs = append(diffs, walkComparisonFieldsAll(actual.Field(index), want.FieldByName(name), fieldPath, action)...)
	}
	return diffs
}

type comparisonFieldAction func(actual, want reflect.Value, path string) error

func (c *StateComparator[Expected, Actual]) walk(want Expected, action comparisonFieldAction) error {
	if c == nil {
		return fmt.Errorf("comparison is nil")
	}
	actualValue := reflect.ValueOf(c.actual).Elem()
	wantValue := reflect.ValueOf(want)
	return walkComparisonFields(actualValue, wantValue, "", action)
}

func walkComparisonFields(actual, want reflect.Value, path string, action comparisonFieldAction) error {
	if !actual.IsValid() || !want.IsValid() {
		return fmt.Errorf("%s: field is missing from comparison or expected state", path)
	}
	if actual.Kind() == reflect.Struct {
		return walkComparisonStruct(actual, want, path, action)
	}
	return action(actual, want, path)
}

func walkComparisonStruct(actual, want reflect.Value, path string, action comparisonFieldAction) error {
	if want.Kind() != reflect.Struct {
		return fmt.Errorf("%s: expected state field is not a struct", path)
	}
	for index := range actual.NumField() {
		name := actual.Type().Field(index).Name
		fieldPath := name
		if path != "" {
			fieldPath = path + "." + name
		}
		if err := walkComparisonFields(actual.Field(index), want.FieldByName(name), fieldPath, action); err != nil {
			return err
		}
	}
	return nil
}

func matchField(actual, want reflect.Value, path string, candidate reflect.Value) error {
	method := actual.MethodByName("MatchWithCandidate")
	if !method.IsValid() || method.Type().NumIn() != 3 || method.Type().NumOut() != 1 || !want.Type().AssignableTo(method.Type().In(1)) {
		return fmt.Errorf("%s: comparison type does not match expected field", path)
	}
	result := method.Call([]reflect.Value{reflect.ValueOf(path), want, candidate})[0]
	if !result.IsNil() {
		return result.Interface().(error)
	}
	return nil
}

// DiagnosticValue returns the observed fields for a mismatch report.
func (c *StateComparator[Expected, Actual]) DiagnosticValue() any {
	if c == nil {
		return nil
	}
	return comparisonDiagnosticValue(reflect.ValueOf(c.actual).Elem())
}

func comparisonDiagnosticValue(value reflect.Value) any {
	if diagnostic, ok := value.Interface().(interface{ DiagnosticValue() any }); ok {
		return diagnostic.DiagnosticValue()
	}
	if value.Kind() != reflect.Struct {
		return value.Interface()
	}
	fields := make(map[string]any, value.NumField())
	for index := range value.NumField() {
		field := value.Type().Field(index)
		if field.IsExported() {
			fields[field.Name] = comparisonDiagnosticValue(value.Field(index))
		}
	}
	return fields
}
