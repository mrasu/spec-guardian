package errorascrashed

import (
	"errors"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestProvider_ObserveResultReturnsErrorPresence(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "success"},
		{name: "failure", err: errors.New("action failed"), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := New(Config{}, nil, []provider.FieldRequest{{Field: provider.ActionCrashedField, Output: provider.ActionCrashedOutputSpec()}})
			require.NoError(t, err)
			value, err := o.ObserveResult(t.Context(), provider.ActionCrashedField, "returned output", tt.err)
			require.NoError(t, err)
			require.Equal(t, tt.want, value)
		})
	}
}

func TestProvider_ObserveResultRejectsUnknownField(t *testing.T) {
	o, err := New(Config{}, map[string]any{}, []provider.FieldRequest{{Field: provider.ActionCrashedField, Config: map[string]any{}, Output: provider.ActionCrashedOutputSpec()}})
	require.NoError(t, err)
	value, err := o.ObserveResult(t.Context(), "unknown", nil, nil)
	require.ErrorContains(t, err, "unknown error-as-crashed field")
	require.Nil(t, value)
}

func TestNew_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		fields []provider.FieldRequest
	}{
		{name: "shared settings", config: map[string]any{"unknown": true}},
		{name: "wrong type", fields: []provider.FieldRequest{{Field: "x", Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindString}}}}},
		{name: "nullable bool", fields: []provider.FieldRequest{{Field: "x", Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindBool, Nullable: true}}}}},
		{name: "empty name", fields: []provider.FieldRequest{{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := New(Config{}, tt.config, tt.fields)
			require.Error(t, err)
			require.Nil(t, o)
		})
	}
}
