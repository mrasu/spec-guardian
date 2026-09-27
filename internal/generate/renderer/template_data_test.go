package renderer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
)

func TestGoTypePromotesMixedNumbersAndValues(t *testing.T) {
	integer := mustJSONValue(t, `1`)
	float := mustJSONValue(t, `1.5`)
	stringValue := mustJSONValue(t, `"ready"`)
	null := mustJSONValue(t, `null`)

	assert.Equal(t, "float64", goType([]any{integer, float}))
	assert.Equal(t, "*string", goType([]any{stringValue, null}))
	assert.Equal(t, "any", goType([]any{null}))
	assert.Equal(t, "*float64", goType([]any{integer, null, float}))
}

func TestUniqueGoNameEscapesOnlyWhenNeeded(t *testing.T) {
	used := make(map[string]struct{})
	assert.Equal(t, "ReadyState", uniqueGoName("ready_state", used))
	assert.Equal(t, "Field_1stValue", uniqueGoName("1st-value", used))
	assert.Equal(t, "ReadyState2", uniqueGoName("ready-state", used))
}

func mustJSONValue(t *testing.T, raw string) any {
	t.Helper()
	object, err := fizzbee.NewJSONObject(json.RawMessage(`{"value":` + raw + `}`))
	require.NoError(t, err)
	return object["value"]
}
