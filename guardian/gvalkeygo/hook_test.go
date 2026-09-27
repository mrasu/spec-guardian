package gvalkeygo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
)

type rejectingFault struct {
	err        error
	operations []string
}

func (f *rejectingFault) BeforeIO(_ context.Context, operation string) error {
	f.operations = append(f.operations, operation)
	return f.err
}

func TestHookInjectsFaultIntoEveryMethod(t *testing.T) {
	injected := errors.New("injected")
	fault := &rejectingFault{err: injected}
	hook := NewHook(fault)

	assert.ErrorIs(t, hook.Do(nil, t.Context(), valkey.Completed{}).Error(), injected)

	multi := hook.DoMulti(nil, t.Context(), valkey.Completed{}, valkey.Completed{})
	require.Len(t, multi, 2)
	for _, result := range multi {
		assert.ErrorIs(t, result.Error(), injected)
	}

	assert.ErrorIs(t, hook.DoCache(nil, t.Context(), valkey.Cacheable{}, 0).Error(), injected)

	multiCache := hook.DoMultiCache(nil, t.Context(), valkey.CacheableTTL{}, valkey.CacheableTTL{})
	require.Len(t, multiCache, 2)
	for _, result := range multiCache {
		assert.ErrorIs(t, result.Error(), injected)
	}

	assert.ErrorIs(t, hook.Receive(nil, t.Context(), valkey.Completed{}, nil), injected)
	stream := hook.DoStream(nil, t.Context(), valkey.Completed{})
	assert.ErrorIs(t, stream.Error(), injected)
	multiStream := hook.DoMultiStream(nil, t.Context(), valkey.Completed{}, valkey.Completed{})
	assert.ErrorIs(t, multiStream.Error(), injected)

	assert.Equal(t, []string{
		"valkey.Do",
		"valkey.DoMulti",
		"valkey.DoCache",
		"valkey.DoMultiCache",
		"valkey.Receive",
		"valkey.DoStream",
		"valkey.DoMultiStream",
	}, fault.operations)
}
