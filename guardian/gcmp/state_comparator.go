package gcmp

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
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
	return walkComparisonFields(state, state, "", validateComparisonField)
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
	if c == nil {
		return fmt.Errorf("comparison is nil")
	}
	actual := reflect.ValueOf(c.actual).Elem()
	want := reflect.ValueOf(*new(Expected))
	return walkComparisonFields(actual, want, "", validateComparisonField)
}

func validateComparisonField(actual, _ reflect.Value, path string) error {
	validator, ok := actual.Interface().(interface{ Validate(string) error })
	if !ok {
		return fmt.Errorf("%s: field is not a comparison", path)
	}
	return validator.Validate(path)
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
		return []string{comparisonShapeDifference(path, "field is missing from comparison or expected state", actual, want)}
	}
	if _, ok := containerAt(actual); ok {
		return matchContainer(actual, want, path, action)
	}
	switch actual.Kind() {
	case reflect.Struct:
		return walkComparisonStructAll(actual, want, path, action)
	case reflect.Map:
		return walkComparisonMapAll(actual, want, path, action)
	case reflect.Slice:
		return walkComparisonSliceAll(actual, want, path, action)
	default:
		if err := action(actual, want, path); err != nil {
			return []string{err.Error()}
		}
		return nil
	}
}

func walkComparisonStructAll(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	if want.Kind() != reflect.Struct {
		return []string{comparisonShapeDifference(path, "expected state field is not a struct", actual, want)}
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

func walkComparisonMapAll(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	if want.Kind() != reflect.Map || actual.Type().Key() != want.Type().Key() {
		return []string{comparisonShapeDifference(path, "map types are incompatible", actual, want)}
	}
	var diffs []string
	if actual.IsNil() != want.IsNil() {
		diffs = append(diffs, comparisonShapeDifference(path, "nil and non-nil maps differ", actual, want))
	}
	for _, key := range sortedMapKeys(actual) {
		keyPath := fmt.Sprintf("%s[%#v]", path, key.Interface())
		expected := want.MapIndex(key)
		if !expected.IsValid() {
			diffs = append(diffs, comparisonShapeDifference(keyPath, "key is missing from expected state", actual, want))
			continue
		}
		diffs = append(diffs, walkComparisonFieldsAll(actual.MapIndex(key), expected, keyPath, action)...)
	}
	for _, key := range sortedMapKeys(want) {
		if !actual.MapIndex(key).IsValid() {
			keyPath := fmt.Sprintf("%s[%#v]", path, key.Interface())
			diffs = append(diffs, comparisonShapeDifference(keyPath, "key is missing from comparison", actual, want))
		}
	}
	return diffs
}

func sortedMapKeys(value reflect.Value) []reflect.Value {
	keys := value.MapKeys()
	slices.SortFunc(keys, func(left, right reflect.Value) int {
		return strings.Compare(fmt.Sprintf("%T:%#v", left.Interface(), left.Interface()), fmt.Sprintf("%T:%#v", right.Interface(), right.Interface()))
	})
	return keys
}

func walkComparisonSliceAll(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	if want.Kind() != reflect.Slice {
		return []string{comparisonShapeDifference(path, "expected state field is not a slice", actual, want)}
	}
	var diffs []string
	if actual.IsNil() != want.IsNil() {
		diffs = append(diffs, comparisonShapeDifference(path, "nil and non-nil slices differ", actual, want))
	}
	if actual.Len() != want.Len() {
		diffs = append(diffs, comparisonShapeDifference(path, fmt.Sprintf("got %d elements, want %d", actual.Len(), want.Len()), actual, want))
	}
	for index := range min(actual.Len(), want.Len()) {
		indexPath := fmt.Sprintf("%s[%d]", path, index)
		diffs = append(diffs, walkComparisonFieldsAll(actual.Index(index), want.Index(index), indexPath, action)...)
	}
	return diffs
}

func comparisonShapeDifference(path, message string, actual, want reflect.Value) string {
	return fmt.Sprintf("%s: %s; got %#v, want %#v", path, message, comparisonDiagnosticValue(actual), comparisonDiagnosticValue(want))
}

type comparisonFieldAction func(actual, want reflect.Value, path string) error

func walkComparisonFields(actual, want reflect.Value, path string, action comparisonFieldAction) error {
	if !actual.IsValid() || !want.IsValid() {
		return fmt.Errorf("%s: field is missing from comparison or expected state", path)
	}
	if container, ok := containerAt(actual); ok {
		return container.Validate(path)
	}
	switch actual.Kind() {
	case reflect.Struct:
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
	case reflect.Map:
		if want.Kind() != reflect.Map || actual.Type().Key() != want.Type().Key() {
			return fmt.Errorf("%s: map types are incompatible", path)
		}
		for _, key := range sortedMapKeys(actual) {
			keyPath := fmt.Sprintf("%s[%#v]", path, key.Interface())
			if err := walkComparisonFields(actual.MapIndex(key), reflect.Zero(want.Type().Elem()), keyPath, action); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if want.Kind() != reflect.Slice {
			return fmt.Errorf("%s: expected state field is not a slice", path)
		}
		for index := range actual.Len() {
			indexPath := fmt.Sprintf("%s[%d]", path, index)
			if err := walkComparisonFields(actual.Index(index), reflect.Zero(want.Type().Elem()), indexPath, action); err != nil {
				return err
			}
		}
	default:
		return action(actual, want, path)
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
	if !value.IsValid() {
		return "<missing>"
	}
	if diagnostic, ok := value.Interface().(interface{ DiagnosticValue() any }); ok {
		return diagnostic.DiagnosticValue()
	}
	switch value.Kind() {
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		if value.Type().Key().Kind() == reflect.String {
			fields := make(map[string]any, value.Len())
			for _, key := range value.MapKeys() {
				fields[key.String()] = comparisonDiagnosticValue(value.MapIndex(key))
			}
			return fields
		}
		fields := make(map[any]any, value.Len())
		for _, key := range value.MapKeys() {
			fields[key.Interface()] = comparisonDiagnosticValue(value.MapIndex(key))
		}
		return fields
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		fields := make([]any, value.Len())
		for index := range fields {
			fields[index] = comparisonDiagnosticValue(value.Index(index))
		}
		return fields
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return comparisonDiagnosticValue(value.Elem())
	case reflect.Struct:
		fields := make(map[string]any, value.NumField())
		for index := range value.NumField() {
			field := value.Type().Field(index)
			if field.IsExported() {
				fields[field.Name] = comparisonDiagnosticValue(value.Field(index))
			}
		}
		return fields
	default:
		return value.Interface()
	}
}
