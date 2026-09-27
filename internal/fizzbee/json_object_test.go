package fizzbee

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJSONObjectParsesNestedValues(t *testing.T) {
	object, err := NewJSONObject(json.RawMessage(`{"integer":1,"float":1.5,"array":[true,null],"object":{"value":"text"}}`))
	require.NoError(t, err)

	assert.Equal(t, ValueKindInteger, ValueKindOf(object["integer"]))
	assert.Equal(t, ValueKindFloat, ValueKindOf(object["float"]))
	assert.Equal(t, ValueKindArray, ValueKindOf(object["array"]))
	assert.Equal(t, ValueKindObject, ValueKindOf(object["object"]))
	assert.JSONEq(t, `{"array":[true,null],"float":1.5,"integer":1,"object":{"value":"text"}}`, mustMarshalJSON(t, object))
}

func TestNewJSONObjectRejectsNonObject(t *testing.T) {
	_, err := NewJSONObject(json.RawMessage(`[1]`))
	assert.ErrorContains(t, err, "want object")
}

func TestNewActionCasesRejectsStateShapeChange(t *testing.T) {
	action, err := NewActionRef("Worker", 1, "Run")
	require.NoError(t, err)
	first := newTestActionCase(t, action, `{"key":1}`, `{"state":"ready"}`)
	changed := newTestActionCase(t, action, `{"other":1}`, `{"state":"done"}`)

	_, err = NewActionCases([]ActionCase{first, changed})
	assert.ErrorContains(t, err, "params key")
}

func newTestActionCase(t *testing.T, action ActionRef, params, fields string) ActionCase {
	t.Helper()
	paramsObject, err := NewJSONObject(json.RawMessage(params))
	require.NoError(t, err)
	fieldsObject, err := NewJSONObject(json.RawMessage(fields))
	require.NoError(t, err)
	role, err := NewRoleState("Worker", paramsObject, fieldsObject)
	require.NoError(t, err)
	input, err := NewInput([]RoleState{role})
	require.NoError(t, err)
	output, err := NewOutput([]RoleState{role})
	require.NoError(t, err)
	actionCase, err := NewActionCase(action, input, []Output{output})
	require.NoError(t, err)
	return actionCase
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
