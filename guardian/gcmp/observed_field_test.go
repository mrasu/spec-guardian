package gcmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObservedFieldMatchesComparison(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		assert.NoError(t, Observed("value").MatchComparison("Fields.Value", Equal("value")))
		err := Observed("actual").MatchComparison("Fields.Value", Equal("allowed"))
		assert.Contains(t, err.Error(), "Fields.Value (-want +got):")
		assert.Contains(t, err.Error(), `"allowed"`)
		assert.Contains(t, err.Error(), `"actual"`)
	})

	t.Run("unobservable", func(t *testing.T) {
		assert.NoError(t, Unobservable[string]("not exposed").MatchComparison("Fields.Value", Equal("allowed")))
	})

	t.Run("ignored", func(t *testing.T) {
		assert.NoError(t, Observed("actual").MatchComparison("Fields.Value", Ignore[string]("not compared")))
	})
}

func TestObservedFieldDiagnosticValue(t *testing.T) {
	assert.Equal(t, "observed", Observed("observed").DiagnosticValue())
	assert.Equal(t, []string{"[UNOBSERVABLE; not exposed]"}, Unobservable[string]("not exposed").DiagnosticValue())
}
