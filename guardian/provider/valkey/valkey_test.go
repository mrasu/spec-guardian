package valkey

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/mrasu/spec-guardian/guardian/gcmp"
	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
	valkeygo "github.com/valkey-io/valkey-go"
)

const (
	testValkeyAddress = "localhost:16379"
	testField         = "Cache.cached_records"
)

func TestObserveIncludesExtraKeysAcrossBatches(t *testing.T) {
	prefix := fmt.Sprintf("sg-valkey-provider:%d:", time.Now().UnixNano())
	want, extra := seedTestRecords(t, prefix, mgetBatchSize+2)

	reader, err := New(Config{}, map[string]any{"address": testValkeyAddress}, []provider.FieldRequest{{
		Field:  testField,
		Config: map[string]any{"keys": prefix + "*"},
		Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString}}},
	}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.CleanupCase(context.Background())) })

	value, err := reader.Observe(t.Context(), testField)
	require.NoError(t, err)
	got, ok := value.(map[string]string)
	require.True(t, ok)

	require.Equal(t, want, got)

	model := maps.Clone(want)
	delete(model, extra)
	require.Error(t, gcmp.Equal(got).Match(testField, model))
}

func seedTestRecords(t *testing.T, prefix string, count int) (map[string]string, string) {
	t.Helper()

	client := newTestValkeyClient(t)

	keys := make([]string, 0, count+1)
	t.Cleanup(func() {
		if len(keys) > 0 {
			require.NoError(t, client.Do(context.Background(), client.B().Del().Key(keys...).Build()).Error())
		}
	})

	want := make(map[string]string, count)
	for index := range count {
		key := fmt.Sprintf("%skey-%03d", prefix, index)
		keys = append(keys, key)
		want[key] = "value"
		require.NoError(t, client.Do(t.Context(), client.B().Set().Key(key).Value("value").Build()).Error())
	}

	extra := prefix + "extra"
	keys = append(keys, extra)
	want[extra] = "unexpected"
	require.NoError(t, client.Do(t.Context(), client.B().Set().Key(extra).Value("unexpected").Build()).Error())

	return want, extra
}

func newTestValkeyClient(t *testing.T) valkeygo.Client {
	t.Helper()

	client, err := valkeygo.NewClient(valkeygo.ClientOption{InitAddress: []string{testValkeyAddress}})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}
