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
