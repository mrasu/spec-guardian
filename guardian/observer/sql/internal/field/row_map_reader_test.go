package field

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/observer"
	"github.com/stretchr/testify/require"
)

func TestNewRowMapReader_RejectsEmptyMessage(t *testing.T) {
	spec := observer.Type{Kind: observer.KindMap, Values: &observer.Type{Kind: observer.KindMessage}}
	reader, err := NewRowMapReader(map[string]any{"key_column": "id"}, spec)
	require.ErrorContains(t, err, "requires message fields")
	require.Nil(t, reader)
}

func TestRowMapReader_KeepsValidatedConfiguration(t *testing.T) {
	spec := observer.Type{Kind: observer.KindMap, Values: &observer.Type{
		Kind: observer.KindMessage,
		Fields: map[string]observer.Type{
			"note": {Kind: observer.KindString, Nullable: true},
		},
	}}
	reader, err := NewRowMapReader(map[string]any{"key_column": "id"}, spec)
	require.NoError(t, err)

	spec.Values.Fields["note"] = observer.Type{Kind: observer.KindString}
	message, err := reader.message("one", []any{nil}, map[string]int{"note": 0})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"note": nil}, message)
}
