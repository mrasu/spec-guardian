package gcmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type containerObservedItem struct {
	Name *ObservedField[string]
	Id   *ObservedField[int64]
}

type containerAllowedItem struct {
	Name *ComparisonField[string]
	Id   *ComparisonField[int64]
}

func TestObservedMessage_NullAndNestedFields(t *testing.T) {
	tests := []struct {
		name     string
		observed *ObservedMessage[containerObservedItem]
		allowed  *ComparisonMessage[containerAllowedItem]
		mismatch string
	}{
		{name: "null", observed: MessageObserved[containerObservedItem](nil), allowed: MessageEqual[containerAllowedItem](nil)},
		{name: "null mismatch", observed: MessageObserved[containerObservedItem](nil), allowed: MessageEqual(&containerAllowedItem{Name: Equal("a"), Id: Equal(int64(1))}), mismatch: "null and non-null messages differ"},
		{name: "nested ignore", observed: MessageObserved(&containerObservedItem{Name: Observed("a"), Id: Observed(int64(9007199254740993))}), allowed: MessageEqual(&containerAllowedItem{Name: Equal("a"), Id: Ignore[int64]("generated ID")})},
		{name: "exact int64", observed: MessageObserved(&containerObservedItem{Name: Observed("a"), Id: Observed(int64(9007199254740993))}), allowed: MessageEqual(&containerAllowedItem{Name: Equal("a"), Id: Equal(int64(9007199254740992))}), mismatch: "Message.Id"},
		{name: "whole message ignore", observed: MessageObserved[containerObservedItem](nil), allowed: MessageIgnore[containerAllowedItem]("implementation detail")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := &struct {
				Message *ObservedMessage[containerObservedItem]
			}{tt.observed}
			allowed := &struct {
				Message *ComparisonMessage[containerAllowedItem]
			}{tt.allowed}
			comparator := NewObservedStateComparator[*struct {
				Message *ComparisonMessage[containerAllowedItem]
			}](observed)
			require.NoError(t, comparator.Validate())
			diffs := comparator.Match(allowed)
			if tt.mismatch == "" {
				require.Empty(t, diffs)
			} else {
				require.Contains(t, diffs[0], tt.mismatch)
			}
		})
	}
}

func TestContainerValidate_RejectsUnsetAndMissingIgnoreReasons(t *testing.T) {
	tests := []struct {
		name  string
		value interface{ Validate(string) error }
		want  string
	}{
		{"nil message observation", (*ObservedMessage[containerObservedItem])(nil), "message observation is not configured"},
		{"unset message observation", &ObservedMessage[containerObservedItem]{}, "message observation is not configured"},
		{"unset message comparison", &ComparisonMessage[containerAllowedItem]{}, "message comparison is not configured"},
		{"unset list observation", &ObservedList[*ObservedField[string]]{}, "list observation is not configured"},
		{"nil list comparison", (*ComparisonList[*ComparisonField[string]])(nil), "list comparison is not configured"},
		{"unset list comparison", &ComparisonList[*ComparisonField[string]]{}, "list comparison is not configured"},
		{"message ignore missing reason", MessageIgnore[containerAllowedItem](""), "Ignore requires a reason"},
		{"list ignore blank reason", ListIgnore[*ComparisonField[string]]("  "), "Ignore requires a reason"},
		{"message missing nested field", MessageObserved(&containerObservedItem{Name: Observed("a")}), "Value.Id"},
		{"list missing nested field", ListObserved([]*ObservedMessage[containerObservedItem]{MessageObserved(&containerObservedItem{Name: Observed("a")})}), "Value[0].Id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.ErrorContains(t, tt.value.Validate("Value"), tt.want) })
	}
}

func TestObservedList_OrderedAndUnorderedMatching(t *testing.T) {
	tests := []struct {
		name     string
		observed []*ObservedField[string]
		allowed  *ComparisonList[*ComparisonField[string]]
		mismatch string
	}{
		{name: "ordered", observed: []*ObservedField[string]{Observed("a"), Observed("b")}, allowed: ListEqual([]*ComparisonField[string]{Equal("a"), Equal("b")})},
		{name: "ordered mismatch", observed: []*ObservedField[string]{Observed("a"), Observed("b")}, allowed: ListEqual([]*ComparisonField[string]{Equal("b"), Equal("a")}), mismatch: "Values[0]"},
		{name: "unordered", observed: []*ObservedField[string]{Observed("a"), Observed("b")}, allowed: ListUnordered([]*ComparisonField[string]{Equal("b"), Equal("a")})},
		{name: "overlapping candidates require reassignment", observed: []*ObservedField[string]{Observed("a"), Observed("b")}, allowed: ListUnordered([]*ComparisonField[string]{Ignore[string]("any value"), Equal("a")})},
		{name: "duplicate count", observed: []*ObservedField[string]{Observed("a"), Observed("a")}, allowed: ListUnordered([]*ComparisonField[string]{Equal("a"), Equal("b")}), mismatch: "no complete one-to-one match"},
		{name: "extra element", observed: []*ObservedField[string]{Observed("a"), Observed("b")}, allowed: ListUnordered([]*ComparisonField[string]{Equal("a")}), mismatch: "got 2 elements, want 1"},
		{name: "missing element", observed: []*ObservedField[string]{Observed("a")}, allowed: ListUnordered([]*ComparisonField[string]{Equal("a"), Equal("b")}), mismatch: "got 1 elements, want 2"},
		{name: "nil", allowed: ListUnordered[*ComparisonField[string]](nil)},
		{name: "empty", observed: []*ObservedField[string]{}, allowed: ListUnordered([]*ComparisonField[string]{})},
		{name: "nil differs from empty", allowed: ListUnordered([]*ComparisonField[string]{}), mismatch: "nil and non-nil slices differ"},
		{name: "whole list ignore", observed: []*ObservedField[string]{Observed("a")}, allowed: ListIgnore[*ComparisonField[string]]("not compared")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := &struct {
				Values *ObservedList[*ObservedField[string]]
			}{ListObserved(tt.observed)}
			allowed := &struct {
				Values *ComparisonList[*ComparisonField[string]]
			}{tt.allowed}
			comparator := NewObservedStateComparator[*struct {
				Values *ComparisonList[*ComparisonField[string]]
			}](observed)
			require.NoError(t, comparator.Validate())
			diffs := comparator.Match(allowed)
			if tt.mismatch == "" {
				require.Empty(t, diffs)
			} else {
				require.Contains(t, diffs[0], tt.mismatch)
			}
		})
	}
}

func TestObservedList_NestedMessagesAndLists(t *testing.T) {
	type observedRow struct {
		Path *ObservedField[string]
		Rows *ObservedList[*ObservedList[*ObservedField[string]]]
	}
	type allowedRow struct {
		Path *ComparisonField[string]
		Rows *ComparisonList[*ComparisonList[*ComparisonField[string]]]
	}
	observed := &struct {
		Files *ObservedList[*ObservedMessage[observedRow]]
	}{
		ListObserved([]*ObservedMessage[observedRow]{MessageObserved(&observedRow{
			Path: Observed("generated"),
			Rows: ListObserved([]*ObservedList[*ObservedField[string]]{
				ListObserved([]*ObservedField[string]{Observed("a"), Observed("b")}),
				ListObserved([]*ObservedField[string]{Observed("c")}),
			}),
		})}),
	}
	allowed := &struct {
		Files *ComparisonList[*ComparisonMessage[allowedRow]]
	}{
		ListUnordered([]*ComparisonMessage[allowedRow]{MessageEqual(&allowedRow{
			Path: Ignore[string]("generated path"),
			Rows: ListUnordered([]*ComparisonList[*ComparisonField[string]]{
				ListEqual([]*ComparisonField[string]{Equal("c")}),
				ListEqual([]*ComparisonField[string]{Equal("a"), Equal("b")}),
			}),
		})}),
	}
	comparator := NewObservedStateComparator[*struct {
		Files *ComparisonList[*ComparisonMessage[allowedRow]]
	}](observed)
	require.NoError(t, comparator.Validate())
	require.Empty(t, comparator.Match(allowed))
	require.Equal(t, map[string]any{"Files": []any{map[string]any{"Path": "generated", "Rows": []any{[]any{"a", "b"}, []any{"c"}}}}}, comparator.DiagnosticValue())
}

type containerLegacyModel struct {
	Message struct {
		Name string
		Id   int64
	}
	Values []string
}

type containerLegacyComparison struct {
	Message *ComparisonMessage[containerAllowedItem]
	Values  *ComparisonList[*ComparisonField[string]]
}

func (c *containerLegacyComparison) ExpectedState() containerLegacyModel {
	return containerLegacyModel{}
}

func TestStateComparator_ContainerComparisonsWithPlainModelValues(t *testing.T) {
	allowed := &containerLegacyComparison{
		Message: MessageEqual(&containerAllowedItem{Name: Equal("a"), Id: Ignore[int64]("generated ID")}),
		Values:  ListUnordered([]*ComparisonField[string]{Equal("a"), Equal("b")}),
	}
	model := containerLegacyModel{Message: struct {
		Name string
		Id   int64
	}{Name: "a", Id: 99}, Values: []string{"b", "a"}}
	comparator := NewStateComparator[containerLegacyModel](allowed)
	require.NoError(t, comparator.Validate())
	require.Empty(t, comparator.Match(model))
	model.Message.Name = "different"
	diffs := comparator.Match(model)
	require.Len(t, diffs, 1)
	require.Contains(t, diffs[0], "Message.Name")
}

func TestObservedList_UnorderedMatchingDoesNotModifyInputs(t *testing.T) {
	first, second := Observed("a"), Observed("b")
	expectedFirst, expectedSecond := Equal("b"), Equal("a")
	actualValues := []*ObservedField[string]{first, second}
	expectedValues := []*ComparisonField[string]{expectedFirst, expectedSecond}
	observed := &struct {
		Values *ObservedList[*ObservedField[string]]
	}{ListObserved(actualValues)}
	allowed := &struct {
		Values *ComparisonList[*ComparisonField[string]]
	}{ListUnordered(expectedValues)}
	comparator := NewObservedStateComparator[*struct {
		Values *ComparisonList[*ComparisonField[string]]
	}](observed)
	require.Empty(t, comparator.Match(allowed))
	require.Equal(t, []*ObservedField[string]{first, second}, actualValues)
	require.Equal(t, []*ComparisonField[string]{expectedFirst, expectedSecond}, expectedValues)
}

func TestObservedMessage_RejectsInvalidObservationEvenWhenIgnored(t *testing.T) {
	observed := &struct {
		Message *ObservedMessage[containerObservedItem]
	}{MessageObserved(&containerObservedItem{Name: Observed("a")})}
	allowed := &struct {
		Message *ComparisonMessage[containerAllowedItem]
	}{MessageIgnore[containerAllowedItem]("not compared")}
	comparator := NewObservedStateComparator[*struct {
		Message *ComparisonMessage[containerAllowedItem]
	}](observed)
	require.ErrorContains(t, comparator.Validate(), "Message.Id")
	diffs := comparator.Match(allowed)
	require.Len(t, diffs, 1)
	require.Contains(t, diffs[0], "Message.Id")
}

func TestObservedMessage_RejectsListComparison(t *testing.T) {
	observed := &struct {
		Value *ObservedMessage[containerObservedItem]
	}{MessageObserved[containerObservedItem](nil)}
	allowed := &struct {
		Value *ComparisonList[*ComparisonField[string]]
	}{ListEqual[*ComparisonField[string]](nil)}
	comparator := NewObservedStateComparator[*struct {
		Value *ComparisonList[*ComparisonField[string]]
	}](observed)
	diffs := comparator.Match(allowed)
	require.Len(t, diffs, 1)
	require.Contains(t, diffs[0], "message and list containers are incompatible")
}

func TestObservedList_UnorderedMismatchReportsNestedFieldPath(t *testing.T) {
	observed := &struct {
		Files *ObservedList[*ObservedMessage[containerObservedItem]]
	}{
		ListObserved([]*ObservedMessage[containerObservedItem]{MessageObserved(&containerObservedItem{Name: Observed("a"), Id: Observed(int64(1))})}),
	}
	allowed := &struct {
		Files *ComparisonList[*ComparisonMessage[containerAllowedItem]]
	}{
		ListUnordered([]*ComparisonMessage[containerAllowedItem]{MessageEqual(&containerAllowedItem{Name: Equal("b"), Id: Equal(int64(1))})}),
	}
	comparator := NewObservedStateComparator[*struct {
		Files *ComparisonList[*ComparisonMessage[containerAllowedItem]]
	}](observed)
	diffs := comparator.Match(allowed)
	require.Len(t, diffs, 3)
	require.Contains(t, diffs[0], "no complete one-to-one match")
	require.Contains(t, diffs[1], "closest expected element is [0]")
	require.Contains(t, diffs[2], "Files[0].Name")
}

func TestObservedMessage_RejectsExtraExpectedField(t *testing.T) {
	type observedMessage struct{ Name *ObservedField[string] }
	observed := &struct {
		Message *ObservedMessage[observedMessage]
	}{MessageObserved(&observedMessage{Name: Observed("a")})}
	allowed := &struct {
		Message *ComparisonMessage[containerAllowedItem]
	}{MessageEqual(&containerAllowedItem{Name: Equal("a"), Id: Equal(int64(1))})}
	comparator := NewObservedStateComparator[*struct {
		Message *ComparisonMessage[containerAllowedItem]
	}](observed)
	diffs := comparator.Match(allowed)
	require.Len(t, diffs, 1)
	require.Contains(t, diffs[0], "Message.Id: field is missing from observed message")
}
