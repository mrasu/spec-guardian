package gcmp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComparisonFieldDiagnosticValue(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		assert.Equal(t, "observed", Equal("observed").DiagnosticValue())
	})
	t.Run("ignore", func(t *testing.T) {
		assert.Equal(t, []string{"[IGNORED; unavailable]"}, Ignore[string]("unavailable").DiagnosticValue())
	})
	t.Run("custom", func(t *testing.T) {
		comparison := Compare("observed", func(string) (bool, string) {
			return true, ""
		})
		assert.Equal(t, "observed", comparison.DiagnosticValue())
	})
	t.Run("candidate", func(t *testing.T) {
		comparison := CompareWithCandidate("observed", func(string, int) (bool, string) {
			return true, ""
		})
		assert.Equal(t, "observed", comparison.DiagnosticValue())
	})
}

func TestCompareWithCandidate(t *testing.T) {
	comparison := CompareWithCandidate("observed", func(want string, candidate int) (bool, string) {
		return want == "expected" && candidate == 3, "candidate did not match"
	})

	assert.NoError(t, comparison.MatchWithCandidate("Fields.Value", "expected", 3))
	assert.EqualError(t, comparison.MatchWithCandidate("Fields.Value", "expected", 4), "Fields.Value: candidate did not match")
	assert.EqualError(t, comparison.MatchWithCandidate("Fields.Value", "expected", "wrong type"), "Fields.Value: candidate has type string, want int")
}

func TestCompareWithCandidateRequiresFunction(t *testing.T) {
	comparison := CompareWithCandidate[string, int]("observed", nil)
	assert.EqualError(t, comparison.Validate("Fields.Value"), "Fields.Value: CompareWithCandidate requires a function")
}

func TestComparisonFieldMatchFormatsPointerValue(t *testing.T) {
	got := "observed"

	err := Equal(&got).Match("Fields.Reader.ResValue", (*string)(nil))

	assert.Contains(t, err.Error(), "Fields.Reader.ResValue (-want +got):")
	assert.Contains(t, err.Error(), "nil")
	assert.Contains(t, err.Error(), `&"observed"`)
}

func TestComparisonFieldMatchFormatsNonPointerValue(t *testing.T) {
	err := Equal("observed").Match("Fields.Reader.Status", "expected")

	assert.Contains(t, err.Error(), "Fields.Reader.Status (-want +got):")
	assert.Contains(t, err.Error(), `"expected"`)
	assert.Contains(t, err.Error(), `"observed"`)
}

func TestComparisonFieldMatchOmitsUnchangedValues(t *testing.T) {
	got := make(map[string]int, 22)
	want := make(map[string]int, 22)
	for index := range 20 {
		key := fmt.Sprintf("same-%02d", index)
		got[key] = index
		want[key] = index
	}
	got["got only"] = 2
	want["want only"] = 3

	err := Equal(got).Match("Fields.Values", want)

	assert.NotContains(t, err.Error(), `"same-10"`)
	assert.Contains(t, err.Error(), `"want only": 3`)
	assert.Contains(t, err.Error(), `"got only":`)
}
