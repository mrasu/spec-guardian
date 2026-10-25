package field

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestNewRowMapReader_RejectsEmptyMessage(t *testing.T) {
	spec := provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindMessage}}
	reader, err := NewRowMapReader(ResultConfig{Type: ResultTypeRowMap, KeyColumn: "id"}, spec)
	require.ErrorContains(t, err, "requires message fields")
	require.Nil(t, reader)
}

func TestRowMapReader_KeepsValidatedConfiguration(t *testing.T) {
	spec := provider.Type{Kind: provider.KindMap, Values: &provider.Type{
		Kind: provider.KindMessage,
		Fields: map[string]provider.Type{
			"note": {Kind: provider.KindString, Nullable: true},
		},
	}}
	reader, err := NewRowMapReader(ResultConfig{Type: ResultTypeRowMap, KeyColumn: "id"}, spec)
	require.NoError(t, err)

	spec.Values.Fields["note"] = provider.Type{Kind: provider.KindString}
	message, err := reader.message("one", []any{nil}, map[string]int{"note": 0})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"note": nil}, message)
}
