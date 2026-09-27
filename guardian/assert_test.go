package guardian

import (
	"errors"
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/gcmp"
	"github.com/stretchr/testify/assert"
)

type testComparison struct {
	want string
}

func NewTestComparison(want string) *testComparison {
	return &testComparison{want: want}
}

func (c *testComparison) Validate() error {
	if c == nil {
		return errors.New("comparison is nil")
	}
	return nil
}

func (c *testComparison) Match(got string) []string {
	if got != c.want {
		return []string{"value mismatch"}
	}
	return nil
}

func TestExecutionComparisonMatchesOutput(t *testing.T) {
	comparison := NewExecutionComparison(NewTestComparison("output"))

	assert.NoError(t, comparison.Validate())
	assert.Empty(t, comparison.Match(NewComparisonState("output")))
	assert.Equal(t, []string{"output: value mismatch"}, comparison.Match(NewComparisonState("different")))
}

func TestFormatEventsAroundInjectedEvent(t *testing.T) {
	events := make([]IOEvent, 25)
	for index := range events {
		events[index] = IOEvent{Number: index + 1, Operation: "operation"}
	}
	events[12].Injected = true

	got := formatEvents(events)
	assert.Contains(t, got, "... 2 earlier I/O events omitted")
	assert.Contains(t, got, "13. operation [fault injected; call not executed]")
	assert.Contains(t, got, "... 2 later I/O events omitted")
}

func TestFormatEventsSuccessPathKeepsLastElevenEvents(t *testing.T) {
	events := make([]IOEvent, 15)
	for index := range events {
		events[index] = IOEvent{Number: index + 1, Operation: "operation"}
	}

	got := formatEvents(events)
	assert.True(t, strings.HasPrefix(got, "... 4 earlier I/O events omitted\n5. operation"))
	assert.True(t, strings.HasSuffix(got, "15. operation"))
}

func TestFormatComparisonMismatchOrdersDiagnostics(t *testing.T) {
	got := formatComparisonMismatch([]string{"difference"}, "actual value", []IOEvent{{Number: 1, Operation: "operation"}})
	eventsIndex := strings.Index(got, "I/O events:")
	diffsIndex := strings.Index(got, "unique mismatches across all allowed states:")
	actualIndex := strings.Index(got, "actual:")
	assert.NotEqual(t, -1, eventsIndex)
	assert.Greater(t, diffsIndex, eventsIndex)
	assert.Greater(t, actualIndex, diffsIndex)
	assert.NotContains(t, got, "allowed:")
}

func TestFormatComparisonMismatchShowsDiagnosticValues(t *testing.T) {
	actual := struct {
		Equal   *gcmp.ComparisonField[string]
		Ignored *gcmp.ComparisonField[string]
		Custom  *gcmp.ComparisonField[string]
	}{
		Equal:   gcmp.Equal("observed"),
		Ignored: gcmp.Ignore[string]("not observable"),
		Custom: gcmp.Compare("custom observed", func(string) (bool, string) {
			return false, "does not match"
		}),
	}

	got := formatComparisonMismatch([]string{"difference"}, actual, nil)
	assert.Contains(t, got, `"Equal": "observed"`)
	assert.Contains(t, got, `"Ignored": [`)
	assert.Contains(t, got, `"[IGNORED; not observable]"`)
	assert.Contains(t, got, `"Custom": "custom observed"`)
}

func TestFormatValueSortsObjectFields(t *testing.T) {
	value := struct {
		Zebra string
		Alpha string
	}{
		Zebra: "zebra",
		Alpha: "alpha",
	}

	got := formatValue(value)
	assert.Less(t, strings.Index(got, `"Alpha"`), strings.Index(got, `"Zebra"`))
}

func TestFormatComparisonMismatchPlacesEventsFirst(t *testing.T) {
	got := formatComparisonMismatch([]string{"first mismatch"}, "actual", nil)

	assert.True(t, strings.HasPrefix(got, "actual state does not match any allowed state:\nI/O events:\n  no supported I/O observed\nunique mismatches across all allowed states:\n  ---diff1\n  first mismatch\n"))
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings([]string{"first", "second", "first"})

	assert.Equal(t, []string{"first", "second"}, got)
}

func TestFormatComparisonMismatchKeepsAllowedDiffsTogether(t *testing.T) {
	got := formatComparisonMismatch([]string{"observed state: first\noutput: second", "output: third"}, "actual", nil)

	assert.Contains(t, got, "---diff1\n  observed state: first\n  output: second\n  ---diff2\n  output: third")
}
