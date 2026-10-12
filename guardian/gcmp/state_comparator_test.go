package gcmp

import (
	"strings"
	"testing"
)

type testExpectedState struct {
	Observed struct {
		Writer struct {
			Result string
			Status string
		}
	}
}

type testActualState struct {
	Observed struct {
		Writer struct {
			Result *ComparisonField[string]
			Status *ComparisonField[string]
		}
	}
}

func NewTestActualState() *testActualState {
	return &testActualState{}
}

func (a *testActualState) ExpectedState() testExpectedState {
	return testExpectedState{}
}

func TestStateComparatorMatchesCandidate(t *testing.T) {
	state := testExpectedState{}
	state.Observed.Writer.Result = "expected"
	state.Observed.Writer.Status = "committed"
	actual := NewTestActualState()
	actual.Observed.Writer.Result = CompareWithCandidate("observed", func(value string, candidate testExpectedState) (bool, string) {
		return value == "expected" && value == candidate.Observed.Writer.Result, "candidate mismatch"
	})
	actual.Observed.Writer.Status = Equal("committed")
	comparator := NewStateComparator[testExpectedState](actual)
	if err := comparator.Validate(); err != nil {
		t.Fatal(err)
	}
	if diffs := comparator.Match(state); len(diffs) != 0 {
		t.Fatal(diffs)
	}
	diagnostic := comparator.DiagnosticValue().(map[string]any)
	observed := diagnostic["Observed"].(map[string]any)["Writer"].(map[string]any)["Result"]
	if observed != "observed" {
		t.Fatalf("DiagnosticValue() = %v, want observed value", observed)
	}
	state.Observed.Writer.Result = "different"
	state.Observed.Writer.Status = "aborted"
	diffs := comparator.Match(state)
	if len(diffs) != 2 || !strings.Contains(diffs[0], "Observed.Writer.Result: candidate mismatch") {
		t.Fatalf("Match() = %v, want field path and mismatch", diffs)
	}
	if !strings.Contains(diffs[1], "Observed.Writer.Status") {
		t.Fatalf("Match() = %v, want all field mismatches", diffs)
	}
	actual.Observed.Writer.Result = nil
	if err := comparator.Validate(); err == nil || !strings.Contains(err.Error(), "Observed.Writer.Result") {
		t.Fatalf("Validate() = %v, want missing field path", err)
	}
}

type nestedExpectedState struct {
	Records map[string][]struct{ Value string }
	Numbers map[int]int
}

type nestedComparisonState struct {
	Records map[string][]struct{ Value *ComparisonField[string] }
	Numbers map[int]*ComparisonField[int]
}

func NewNestedComparisonState() *nestedComparisonState {
	return &nestedComparisonState{}
}

func (s *nestedComparisonState) ExpectedState() nestedExpectedState {
	return nestedExpectedState{}
}

func TestStateComparatorMatchesMapAndSliceFields(t *testing.T) {
	comparison := NewNestedComparisonState()
	comparison.Records = map[string][]struct{ Value *ComparisonField[string] }{
		"first": {{Value: Equal("expected")}, {Value: Ignore[string]("variable value")}},
	}
	comparison.Numbers = map[int]*ComparisonField[int]{1: Equal(1), 2: Ignore[int]("variable value")}
	comparator := NewStateComparator[nestedExpectedState](comparison)
	if err := comparator.Validate(); err != nil {
		t.Fatal(err)
	}

	want := nestedExpectedState{Records: map[string][]struct{ Value string }{
		"first": {{Value: "expected"}, {Value: "different"}},
	}, Numbers: map[int]int{1: 1, 2: 3}}
	if diffs := comparator.Match(want); len(diffs) != 0 {
		t.Fatalf("Match() = %v, want no differences", diffs)
	}
	if got := comparator.DiagnosticValue().(map[string]any)["Records"].(map[string]any)["first"].([]any)[1].(map[string]any)["Value"]; got == nil {
		t.Fatal("DiagnosticValue() omitted the nested field")
	}

	want.Records["first"][0].Value = "wrong"
	if diffs := comparator.Match(want); len(diffs) != 1 || !strings.Contains(diffs[0], `Records["first"][0].Value`) {
		t.Fatalf("Match() = %v, want the nested field path", diffs)
	}
}

func TestStateComparatorChecksMapKeysAndSliceLength(t *testing.T) {
	comparison := NewNestedComparisonState()
	comparison.Records = map[string][]struct{ Value *ComparisonField[string] }{
		"first": {{Value: Equal("value")}},
		"extra": {{Value: Equal("value")}},
	}
	comparator := NewStateComparator[nestedExpectedState](comparison)
	want := nestedExpectedState{Records: map[string][]struct{ Value string }{
		"first":   {{Value: "value"}, {Value: "another"}},
		"missing": {{Value: "value"}},
	}}
	diffs := comparator.Match(want)
	allDiffs := strings.Join(diffs, "\n")
	if len(diffs) != 3 || !strings.Contains(allDiffs, `Records["first"]: got 1 elements, want 2; got `) || !strings.Contains(allDiffs, `"value"`) || !strings.Contains(allDiffs, `"another"`) || !strings.Contains(allDiffs, `Records["extra"]: key is missing`) || !strings.Contains(allDiffs, `Records["missing"]: key is missing`) {
		t.Fatalf("Match() = %v, want length and key differences", diffs)
	}
}

func TestStateComparatorValidatesNestedFields(t *testing.T) {
	comparison := NewNestedComparisonState()
	comparison.Records = map[string][]struct{ Value *ComparisonField[string] }{
		"first": {{Value: nil}},
	}
	comparator := NewStateComparator[nestedExpectedState](comparison)
	if err := comparator.Validate(); err == nil || !strings.Contains(err.Error(), `Records["first"][0].Value`) {
		t.Fatalf("Validate() = %v, want the nested field path", err)
	}
}

func TestStateComparatorDistinguishesNilAndEmptyCollections(t *testing.T) {
	comparison := NewNestedComparisonState()
	comparison.Records = map[string][]struct{ Value *ComparisonField[string] }{"first": {}}
	comparison.Numbers = map[int]*ComparisonField[int]{}
	comparator := NewStateComparator[nestedExpectedState](comparison)
	want := nestedExpectedState{Records: map[string][]struct{ Value string }{"first": nil}}
	diffs := comparator.Match(want)
	if len(diffs) != 2 || !strings.Contains(strings.Join(diffs, "\n"), `Records["first"]: nil and non-nil slices differ; got `) || !strings.Contains(strings.Join(diffs, "\n"), "Numbers: nil and non-nil maps differ; got ") {
		t.Fatalf("Match() = %v, want nil and empty collection differences with values", diffs)
	}
}

type nestedObservedState struct {
	Records map[string][]struct{ Value *ObservedField[string] }
}

func TestObservedStateComparatorMatchesMapAndSliceFields(t *testing.T) {
	observed := &nestedObservedState{Records: map[string][]struct{ Value *ObservedField[string] }{
		"first": {{Value: Observed("actual")}},
	}}
	allowed := NewNestedComparisonState()
	allowed.Records = map[string][]struct{ Value *ComparisonField[string] }{
		"first": {{Value: Ignore[string]("variable value")}},
	}
	comparator := NewObservedStateComparator[*nestedComparisonState](observed)
	if err := comparator.Validate(); err != nil {
		t.Fatal(err)
	}
	if diffs := comparator.Match(allowed); len(diffs) != 0 {
		t.Fatalf("Match() = %v, want ignored nested field", diffs)
	}

	allowed.Records["first"][0].Value = Equal("expected")
	if diffs := comparator.Match(allowed); len(diffs) != 1 || !strings.Contains(diffs[0], `Records["first"][0].Value`) {
		t.Fatalf("Match() = %v, want nested field mismatch", diffs)
	}
}
