package errorascrashed

import (
	"errors"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/observer"
	"github.com/stretchr/testify/require"
)

func TestObserver_ObserveReturnsErrorPresence(t *testing.T) {
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
			o, err := New(Config{}, nil, []observer.FieldRequest{{Field: observer.ActionCrashedField, Output: observer.ActionCrashedOutputSpec()}})
			require.NoError(t, err)
			value, err := o.Observe(t.Context(), observer.ActionCrashedField, "returned output", tt.err)
			require.NoError(t, err)
			require.Equal(t, tt.want, value)
		})
	}
}

func TestObserver_ObserveRejectsUnknownField(t *testing.T) {
	o, err := New(Config{}, map[string]any{}, []observer.FieldRequest{{Field: observer.ActionCrashedField, Config: map[string]any{}, Output: observer.ActionCrashedOutputSpec()}})
	require.NoError(t, err)
	value, err := o.Observe(t.Context(), "unknown", nil, nil)
	require.ErrorContains(t, err, "unknown error-as-crashed field")
	require.Nil(t, value)
}

func TestNew_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		fields []observer.FieldRequest
	}{
		{name: "shared settings", config: map[string]any{"unknown": true}},
		{name: "wrong type", fields: []observer.FieldRequest{{Field: "x", Output: observer.OutputSpec{Type: observer.Type{Kind: observer.KindString}}}}},
		{name: "nullable bool", fields: []observer.FieldRequest{{Field: "x", Output: observer.OutputSpec{Type: observer.Type{Kind: observer.KindBool, Nullable: true}}}}},
		{name: "empty name", fields: []observer.FieldRequest{{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := New(Config{}, tt.config, tt.fields)
			require.Error(t, err)
			require.Nil(t, o)
		})
	}
}
