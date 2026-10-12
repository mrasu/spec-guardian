package gcmp

import (
	"fmt"
	"reflect"
	"strings"
)

type containerKind uint8

const (
	containerMessage containerKind = iota
	containerList
)

// Containers erase only the message/list element type at the recursive comparison boundary.
type comparisonContainer interface {
	Validate(string) error
	containerKind() containerKind
	containerValue() reflect.Value
}

type allowedContainer interface {
	comparisonContainer
	containerComparison() (ignored, ignoreOrder bool)
}

func validateIgnoredReason(path, ignoredReason string) error {
	if strings.TrimSpace(ignoredReason) == "" {
		return fmt.Errorf("%s: Ignore requires a reason", path)
	}
	return nil
}

func validateContainerValue(value reflect.Value, path string) error {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	return walkComparisonFields(value, value, path, validateComparisonField)
}

func containerAt(value reflect.Value) (comparisonContainer, bool) {
	if !value.IsValid() || !value.CanInterface() {
		return nil, false
	}
	container, ok := value.Interface().(comparisonContainer)
	return container, ok
}

func matchContainer(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	observed, _ := containerAt(actual)
	if err := observed.Validate(path); err != nil {
		return []string{err.Error()}
	}
	expected, ok := containerAt(want)
	var allowed allowedContainer
	if ok {
		if observed.containerKind() != expected.containerKind() {
			return []string{fmt.Sprintf("%s: message and list containers are incompatible", path)}
		}
		var isAllowed bool
		allowed, isAllowed = expected.(allowedContainer)
		if !isAllowed {
			return []string{fmt.Sprintf("%s: expected container is not a comparison", path)}
		}
		if _, actualIsAllowed := observed.(allowedContainer); actualIsAllowed {
			return []string{fmt.Sprintf("%s: actual container is not an observation", path)}
		}
		if err := expected.Validate(path); err != nil {
			return []string{err.Error()}
		}
		want = expected.containerValue()
	} else {
		// The legacy comparator compares allowed wrappers with plain model values.
		allowed, ok = observed.(allowedContainer)
		if !ok {
			return []string{fmt.Sprintf("%s: expected container is not a comparison", path)}
		}
	}
	ignored, ignoreOrder := allowed.containerComparison()
	if ignored {
		return nil
	}
	actual = observed.containerValue()
	switch observed.containerKind() {
	case containerMessage:
		if want.Kind() == reflect.Struct {
			if actual.IsNil() {
				return []string{comparisonShapeDifference(path, "null and non-null messages differ", actual, want)}
			}
			return matchMessageFields(actual.Elem(), want, path, action)
		}
		if want.Kind() != reflect.Pointer || want.Type().Elem().Kind() != reflect.Struct {
			return []string{comparisonShapeDifference(path, "expected message is not a pointer to a struct", actual, want)}
		}
		if actual.IsNil() != want.IsNil() {
			return []string{comparisonShapeDifference(path, "null and non-null messages differ", actual, want)}
		}
		if actual.IsNil() {
			return nil
		}
		return matchMessageFields(actual.Elem(), want.Elem(), path, action)
	case containerList:
		if !ignoreOrder {
			return walkComparisonSliceAll(actual, want, path, action)
		}
		return matchUnorderedElements(actual, want, path, action)
	default:
		panic("unsupported container kind")
	}
}

func matchMessageFields(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	diffs := walkComparisonFieldsAll(actual, want, path, action)
	for i := range want.NumField() {
		name := want.Type().Field(i).Name
		if !actual.FieldByName(name).IsValid() {
			fieldPath := name
			if path != "" {
				fieldPath = path + "." + name
			}
			diffs = append(diffs, comparisonShapeDifference(fieldPath, "field is missing from observed message", actual, want))
		}
	}
	return diffs
}

// Augmenting paths find a complete matching even when ignored fields create overlapping candidates.
func matchUnorderedElements(actual, want reflect.Value, path string, action comparisonFieldAction) []string {
	if want.Kind() != reflect.Slice {
		return []string{comparisonShapeDifference(path, "expected state field is not a slice", actual, want)}
	}
	if actual.IsNil() != want.IsNil() {
		return []string{comparisonShapeDifference(path, "nil and non-nil slices differ", actual, want)}
	}
	if actual.Len() != want.Len() {
		return []string{comparisonShapeDifference(path, fmt.Sprintf("got %d elements, want %d", actual.Len(), want.Len()), actual, want)}
	}
	candidates := make([][]int, actual.Len())
	closestDiffs := make([][]string, actual.Len())
	closestIndices := make([]int, actual.Len())
	for i := range actual.Len() {
		for j := range want.Len() {
			diffs := walkComparisonFieldsAll(actual.Index(i), want.Index(j), fmt.Sprintf("%s[%d]", path, i), action)
			if len(diffs) == 0 {
				candidates[i] = append(candidates[i], j)
			}
			if closestDiffs[i] == nil || len(diffs) < len(closestDiffs[i]) {
				closestDiffs[i], closestIndices[i] = diffs, j
			}
		}
	}
	matched := make([]int, want.Len())
	for i := range matched {
		matched[i] = -1
	}
	var assign func(int, []bool) bool
	assign = func(i int, visited []bool) bool {
		for _, j := range candidates[i] {
			if visited[j] {
				continue
			}
			visited[j] = true
			if matched[j] == -1 || assign(matched[j], visited) {
				matched[j] = i
				return true
			}
		}
		return false
	}
	for i := range actual.Len() {
		if !assign(i, make([]bool, want.Len())) {
			diffs := []string{comparisonShapeDifference(fmt.Sprintf("%s[%d]", path, i), "unordered list has no complete one-to-one match", actual, want)}
			if len(candidates[i]) == 0 && len(closestDiffs[i]) > 0 {
				diffs = append(diffs, fmt.Sprintf("%s[%d]: closest expected element is [%d]", path, i, closestIndices[i]))
				diffs = append(diffs, closestDiffs[i]...)
			}
			return diffs
		}
	}
	return nil
}
