package guardian

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/gcmp"
)

// SgFizzbeeMetadata contains metadata read from generated FizzBee case JSON.
type SgFizzbeeMetadata struct {
	ActionCrashed bool `json:"action_crashed"`
}

// SgFetchedMetadata contains raw metadata values returned by result observers.
type SgFetchedMetadata struct {
	ActionCrashed bool
}

// SgObservedMetadata contains metadata observed through result observers.
type SgObservedMetadata struct {
	ActionCrashed *gcmp.ObservedField[bool]
}

// SgComparisonMetadata configures metadata comparisons using the standard field mechanism.
type SgComparisonMetadata struct {
	ActionCrashed *gcmp.ComparisonField[bool]
}

// AssertAllowed reports a failure unless the complete observed state matches one allowed state.
// Allowed and Observed must be pointers to structs containing comparison and observed fields.
func AssertAllowed[Allowed, Observed any](
	t testing.TB,
	allowed []Allowed,
	observed Observed,
	events []IOEvent,
) {
	t.Helper()
	actual := gcmp.NewObservedStateComparator[Allowed](observed)
	if err := actual.Validate(); err != nil {
		t.Fatal("incomplete execution comparison:", err)
	}
	var allowedDiffs []string
	for _, candidate := range allowed {
		candidateDiffs := actual.Match(candidate)
		if len(candidateDiffs) == 0 {
			return
		}
		allowedDiffs = append(allowedDiffs, strings.Join(candidateDiffs, "\n"))
	}
	t.Error(formatComparisonMismatch(uniqueStrings(allowedDiffs), actual, events))
}

func uniqueStrings(values []string) []string {
	unique := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func formatComparisonMismatch(diffs []string, actual any, events []IOEvent) string {
	return fmt.Sprintf(
		`actual state does not match any allowed state:
I/O events:
%s
unique mismatches across all allowed states:
%s
actual:
%s`,
		indent(formatEvents(events), 2),
		indent(formatComparisonDiffs(diffs), 2),
		indent(formatValue(diagnosticValue(actual)), 2),
	)
}

func formatComparisonDiffs(diffs []string) string {
	var builder strings.Builder
	for index, diff := range diffs {
		fmt.Fprintf(&builder, "---diff%d\n%s", index+1, diff)
		if index+1 < len(diffs) {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func diagnosticValue(value any) any {
	return diagnosticReflectValue(reflect.ValueOf(value))
}

func diagnosticReflectValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	if diagnostic, ok := diagnosticMethod(value); ok {
		return diagnostic.Call(nil)[0].Interface()
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return diagnosticReflectValue(value.Elem())
	case reflect.Struct:
		fields := make(map[string]any)
		valueType := value.Type()
		for index := range value.NumField() {
			field := valueType.Field(index)
			if !field.IsExported() {
				continue
			}
			fields[field.Name] = diagnosticReflectValue(value.Field(index))
		}
		return fields
	default:
		return value.Interface()
	}
}

func diagnosticMethod(value reflect.Value) (reflect.Value, bool) {
	method := value.MethodByName("DiagnosticValue")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return reflect.Value{}, false
	}
	return method, true
}

func formatValue(value any) string {
	var builder strings.Builder
	writeValue(&builder, reflect.ValueOf(value), 0)
	return builder.String()
}

func writeValue(builder *strings.Builder, value reflect.Value, depth int) {
	if !value.IsValid() {
		builder.WriteString("nil")
		return
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			builder.WriteString("nil")
			return
		}
		writeValue(builder, value.Elem(), depth)
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		fields := make([]int, 0, value.NumField())
		valueType := value.Type()
		for index := range value.NumField() {
			if valueType.Field(index).IsExported() {
				fields = append(fields, index)
			}
		}
		sort.Slice(fields, func(first, second int) bool {
			return valueType.Field(fields[first]).Name < valueType.Field(fields[second]).Name
		})
		writeObject(builder, len(fields), depth, func(index int) {
			field := valueType.Field(fields[index])
			builder.WriteString(strconv.Quote(field.Name))
			builder.WriteString(": ")
			writeValue(builder, value.Field(fields[index]), depth+1)
		})
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(first, second int) bool {
			return fmt.Sprint(keys[first].Interface()) < fmt.Sprint(keys[second].Interface())
		})
		writeObject(builder, len(keys), depth, func(index int) {
			writeValue(builder, keys[index], depth+1)
			builder.WriteString(": ")
			writeValue(builder, value.MapIndex(keys[index]), depth+1)
		})
	case reflect.Array, reflect.Slice:
		writeList(builder, value.Len(), depth, func(index int) {
			writeValue(builder, value.Index(index), depth+1)
		})
	case reflect.String:
		builder.WriteString(strconv.Quote(value.String()))
	default:
		fmt.Fprint(builder, value.Interface())
	}
}

func writeObject(builder *strings.Builder, length, depth int, writeField func(int)) {
	if length == 0 {
		builder.WriteString("{}")
		return
	}
	builder.WriteString("{\n")
	for index := range length {
		writeIndent(builder, depth+1)
		writeField(index)
		if index+1 < length {
			builder.WriteByte(',')
		}
		builder.WriteByte('\n')
	}
	writeIndent(builder, depth)
	builder.WriteByte('}')
}

func writeList(builder *strings.Builder, length, depth int, writeElement func(int)) {
	if length == 0 {
		builder.WriteString("[]")
		return
	}
	builder.WriteString("[\n")
	for index := range length {
		writeIndent(builder, depth+1)
		writeElement(index)
		if index+1 < length {
			builder.WriteByte(',')
		}
		builder.WriteByte('\n')
	}
	writeIndent(builder, depth)
	builder.WriteByte(']')
}

func writeIndent(builder *strings.Builder, depth int) {
	builder.WriteString(strings.Repeat("  ", depth))
}

func formatEvents(events []IOEvent) string {
	if len(events) == 0 {
		return "no supported I/O observed\n[no fault injected]"
	}

	const contextSize = 10
	start, end := eventRange(events, contextSize)

	var builder strings.Builder
	injected := false
	if start > 0 {
		fmt.Fprintf(&builder, "... %d earlier I/O events omitted\n", start)
	}
	for _, event := range events[start:end] {
		fmt.Fprintf(&builder, "%d. %s", event.Number, event.Operation)
		if event.Injected {
			injected = true
			builder.WriteString(" [fault injected; call not executed]")
		}
		builder.WriteByte('\n')
	}
	if end < len(events) {
		fmt.Fprintf(&builder, "... %d later I/O events omitted\n", len(events)-end)
	}
	if !injected {
		builder.WriteString("[no fault injected]\n")
	}
	return strings.TrimSuffix(builder.String(), "\n")
}

func eventRange(events []IOEvent, contextSize int) (int, int) {
	for index, event := range events {
		if event.Injected {
			return max(index-contextSize, 0), min(index+contextSize+1, len(events))
		}
	}
	return max(len(events)-contextSize-1, 0), len(events)
}

func indent(value string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	return prefix + strings.ReplaceAll(strings.TrimSuffix(value, "\n"), "\n", "\n"+prefix)
}
