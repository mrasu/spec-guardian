package valkey

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_ValidatesKeyspaces(t *testing.T) {
	tests := []struct {
		name        string
		spaces      any
		fieldConfig map[string]any
		spec        provider.Type
		cause       string
	}{
		{name: "null", spaces: nil, cause: "/keyspaces"},
		{name: "not a list", spaces: "records", cause: "keyspaces"},
		{name: "missing name", spaces: []any{map[string]any{"prefix": "", "data_type": "string"}}, cause: "missing property 'name'"},
		{name: "unsupported type", spaces: []any{map[string]any{"name": "records", "prefix": "", "data_type": "list"}}, cause: "/data_type"},
		{name: "duplicate", spaces: []any{testSpace(""), testSpace("")}, cause: "duplicate"},
		{name: "unknown reference", spaces: []any{testSpace("")}, fieldConfig: map[string]any{"keyspace": "unknown"}, cause: "unknown keyspace"},
		{name: "empty reference", spaces: []any{testSpace("")}, fieldConfig: map[string]any{"keyspace": ""}, cause: "/keyspace"},
		{name: "ambiguous", spaces: []any{testSpace("")}, fieldConfig: map[string]any{"keyspace": "records", "keys": "*"}, cause: "oneOf"},
		{name: "wrong model type", spaces: []any{testSpace("")}, spec: provider.Type{Kind: provider.KindList}, cause: "map"},
		{name: "nullable values", spaces: []any{testSpace("")}, spec: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString, Nullable: true}}, cause: "non-nullable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := testKeyspaceField()
			if tt.fieldConfig != nil {
				field.Config = tt.fieldConfig
			}
			if tt.spec.Kind != "" {
				field.Output.Type = tt.spec
			}
			got, err := New(Config{}, map[string]any{"address": testValkeyAddress, "keyspaces": tt.spaces}, []provider.FieldRequest{field})
			require.ErrorContains(t, err, tt.cause)
			require.Nil(t, got)
		})
	}
}

func TestProvider_SeedAndIsolateKeyspace(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-seed:%d:", time.Now().UnixNano())
	outside := prefix + "outside"
	require.NoError(t, client.Do(t.Context(), client.B().Set().Key(outside).Value("untouched").Build()).Error())
	t.Cleanup(func() {
		require.NoError(t, client.Do(context.Background(), client.B().Del().Key(outside).Build()).Error())
	})
	p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix + "records:")}}, []provider.FieldRequest{testKeyspaceField()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
	stale := prefix + "records:stale"
	require.NoError(t, client.Do(t.Context(), client.B().Lpush().Key(stale).Element("wrong type").Build()).Error())
	require.NoError(t, p.PrepareAttempt(t.Context()))
	staleValues, err := client.Do(t.Context(), client.B().Lrange().Key(stale).Start(0).Stop(-1).Build()).AsStrSlice()
	require.NoError(t, err)
	require.Equal(t, []string{"wrong type"}, staleValues)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	observed, err := p.Observe(t.Context(), testField)
	require.NoError(t, err)
	require.Equal(t, map[string]string{}, observed)
	want := map[string]string{}
	for i := range mgetBatchSize + 2 {
		want[fmt.Sprint(i)] = fmt.Sprint(i)
	}
	request := provider.SeedRequest{Values: []provider.SeedValue{{Field: testField, Value: want}}}
	result, err := p.Seed(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, provider.SeedResult{Values: request.Values}, result)
	observed, err = p.Observe(t.Context(), testField)
	require.NoError(t, err)
	require.Equal(t, want, observed)
	for key, expected := range want {
		actual, err := client.Do(t.Context(), client.B().Get().Key(prefix+"records:"+key).Build()).ToString()
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
	extra := prefix + "records:extra"
	require.NoError(t, client.Do(t.Context(), client.B().Set().Key(extra).Value("extra").Build()).Error())
	observed, err = p.Observe(t.Context(), testField)
	require.NoError(t, err)
	want["extra"] = "extra"
	require.Equal(t, want, observed)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	require.NoError(t, p.PrepareAttempt(t.Context()))
	empty := provider.SeedRequest{Values: []provider.SeedValue{{Field: testField, Value: map[string]string{}}}}
	result, err = p.Seed(t.Context(), empty)
	require.NoError(t, err)
	require.Equal(t, provider.SeedResult{Values: empty.Values}, result)
	observed, err = p.Observe(t.Context(), testField)
	require.NoError(t, err)
	require.Equal(t, map[string]string{}, observed)
	value, err := client.Do(t.Context(), client.B().Get().Key(outside).Build()).ToString()
	require.NoError(t, err)
	require.Equal(t, "untouched", value)
}

func TestProvider_ObserveRejectsUnexpectedTypeAndCleanupRemovesIt(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-type:%d:", time.Now().UnixNano())
	p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix)}}, []provider.FieldRequest{testKeyspaceField()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
	key := prefix + "list"
	require.NoError(t, client.Do(t.Context(), client.B().Lpush().Key(key).Element("value").Build()).Error())
	got, err := p.Observe(t.Context(), testField)
	require.ErrorContains(t, err, `has type "list"`)
	require.Nil(t, got)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	count, err := client.Do(t.Context(), client.B().Exists().Key(key).Build()).ToInt64()
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestProvider_SeedRejectsInvalidRequest(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-invalid:%d:", time.Now().UnixNano())
	p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix)}}, []provider.FieldRequest{testKeyspaceField()})
	require.NoError(t, err)
	tests := []struct {
		name   string
		values []provider.SeedValue
		cause  string
	}{
		{name: "missing field", cause: "requires all"},
		{name: "unknown field", values: []provider.SeedValue{{Field: "unknown", Value: map[string]string{}}}, cause: "unknown or duplicate"},
		{name: "invalid value", values: []provider.SeedValue{{Field: testField, Value: "bad"}}, cause: "Valkey seed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Seed(t.Context(), provider.SeedRequest{Values: tt.values})
			require.ErrorContains(t, err, tt.cause)
			require.Equal(t, provider.SeedResult{}, result)
			observed, err := p.Observe(t.Context(), testField)
			require.NoError(t, err)
			require.Equal(t, map[string]string{}, observed)
		})
	}
}

func testSpace(prefix string) map[string]any {
	return map[string]any{"name": "records", "prefix": prefix, "data_type": "string"}
}
func testKeyspaceField() provider.FieldRequest {
	return provider.FieldRequest{Field: testField, Config: map[string]any{"keyspace": "records"}, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString}}}}
}

func TestProvider_SeedPreservesOrderAndObservationOnlyValues(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-mixed:%d:", time.Now().UnixNano())
	key := prefix + "manual"
	require.NoError(t, client.Do(t.Context(), client.B().Set().Key(key).Value("manual").Build()).Error())
	t.Cleanup(func() { require.NoError(t, client.Do(context.Background(), client.B().Del().Key(key).Build()).Error()) })
	manual := testKeyspaceField()
	manual.Field = "Cache.manual"
	manual.Config = map[string]any{"keys": key}
	p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix + "seed:")}}, []provider.FieldRequest{testKeyspaceField(), manual})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
	require.NoError(t, p.PrepareAttempt(t.Context()))
	request := provider.SeedRequest{Values: []provider.SeedValue{
		{Field: manual.Field, Value: map[string]string{key: "manual"}},
		{Field: testField, Value: map[string]string{"one": "seeded"}},
	}}
	result, err := p.Seed(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, provider.SeedResult{Values: request.Values}, result)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	got, err := p.Observe(t.Context(), manual.Field)
	require.NoError(t, err)
	require.Equal(t, request.Values[0].Value, got)
	duplicate := provider.SeedRequest{Values: []provider.SeedValue{request.Values[1], request.Values[1]}}
	result, err = p.Seed(t.Context(), duplicate)
	require.ErrorContains(t, err, "duplicate")
	require.Equal(t, provider.SeedResult{}, result)
}

func TestProvider_SeedRequiresConsistentOverlappingKeyspaces(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-overlap:%d:", time.Now().UnixNano())
	other := testKeyspaceField()
	other.Field = "Cache.other"
	p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix)}}, []provider.FieldRequest{testKeyspaceField(), other})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
	tests := []struct {
		name     string
		other    map[string]string
		conflict bool
	}{
		{name: "same values", other: map[string]string{"one": "one"}},
		{name: "different values", other: map[string]string{"one": "different"}, conflict: true},
		{name: "missing key", other: map[string]string{}, conflict: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
			require.NoError(t, p.PrepareAttempt(t.Context()))
			result, err := p.Seed(t.Context(), provider.SeedRequest{Values: []provider.SeedValue{
				{Field: testField, Value: map[string]string{"one": "one"}},
				{Field: other.Field, Value: tt.other},
			}})
			if tt.conflict {
				require.ErrorContains(t, err, "overlapping keyspaces")
				require.Equal(t, provider.SeedResult{}, result)
				observed, err := p.Observe(t.Context(), testField)
				require.NoError(t, err)
				require.Equal(t, map[string]string{}, observed)
			} else {
				require.NoError(t, err)
				for _, field := range []string{testField, other.Field} {
					observed, err := p.Observe(t.Context(), field)
					require.NoError(t, err)
					require.Equal(t, tt.other, observed)
				}
			}
		})
	}
}

func TestProvider_UsesLiteralPrefixForSeedObservationAndCleanup(t *testing.T) {
	tests := []string{"records:", "no-delimiter", "*", "?", "[ab]", `\`}
	for _, suffix := range tests {
		t.Run(suffix, func(t *testing.T) {
			client := newTestValkeyClient(t)
			base := fmt.Sprintf("sg-valkey-literal:%d:", time.Now().UnixNano())
			prefix := base + suffix
			outside := base + "outside"
			require.NoError(t, client.Do(t.Context(), client.B().Set().Key(outside).Value("untouched").Build()).Error())
			t.Cleanup(func() {
				require.NoError(t, client.Do(context.Background(), client.B().Del().Key(outside).Build()).Error())
			})
			p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix)}}, []provider.FieldRequest{testKeyspaceField()})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
			require.NoError(t, p.PrepareAttempt(t.Context()))
			model := map[string]string{"cache": "value", "": "empty key", prefix + "cache": "prefix in model key"}
			request := provider.SeedRequest{Values: []provider.SeedValue{{Field: testField, Value: model}}}
			result, err := p.Seed(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, provider.SeedResult{Values: request.Values}, result)
			for key, expected := range model {
				value, err := client.Do(t.Context(), client.B().Get().Key(prefix+key).Build()).ToString()
				require.NoError(t, err)
				require.Equal(t, expected, value)
			}
			observed, err := p.Observe(t.Context(), testField)
			require.NoError(t, err)
			require.Equal(t, model, observed)
			require.NoError(t, client.Do(t.Context(), client.B().Set().Key(prefix+"extra").Value("unexpected").Build()).Error())
			observed, err = p.Observe(t.Context(), testField)
			require.NoError(t, err)
			require.Equal(t, map[string]string{"cache": "value", "": "empty key", prefix + "cache": "prefix in model key", "extra": "unexpected"}, observed)
			require.NoError(t, p.CleanupAttempt(t.Context()))
			for _, key := range []string{"cache", "", prefix + "cache", "extra"} {
				count, err := client.Do(t.Context(), client.B().Exists().Key(prefix+key).Build()).ToInt64()
				require.NoError(t, err)
				require.Zero(t, count)
			}
			value, err := client.Do(t.Context(), client.B().Get().Key(outside).Build()).ToString()
			require.NoError(t, err)
			require.Equal(t, "untouched", value)
		})
	}
}

func TestProvider_SeedComparesOverlappingPrefixesUsingConcreteKeys(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-nested:%d:", time.Now().UnixNano())
	outer := testKeyspaceField()
	inner := testKeyspaceField()
	inner.Field = "Cache.other"
	inner.Config = map[string]any{"keyspace": "nested"}
	nested := map[string]any{"name": "nested", "prefix": prefix + "nested:", "data_type": "string"}
	p, err := New(Config{Client: client}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix), nested}}, []provider.FieldRequest{outer, inner})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
	request := provider.SeedRequest{Values: []provider.SeedValue{
		{Field: outer.Field, Value: map[string]string{"nested:cache": "value"}},
		{Field: inner.Field, Value: map[string]string{"cache": "value"}},
	}}
	result, err := p.Seed(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, provider.SeedResult{Values: request.Values}, result)
	for _, field := range request.Values {
		observed, err := p.Observe(t.Context(), field.Field)
		require.NoError(t, err)
		require.Equal(t, field.Value, observed)
	}
	require.NoError(t, p.CleanupAttempt(t.Context()))
	request.Values[1].Value = map[string]string{"cache": "conflict"}
	result, err = p.Seed(t.Context(), request)
	require.ErrorContains(t, err, "overlapping keyspaces")
	require.Equal(t, provider.SeedResult{}, result)
	observed, err := p.Observe(t.Context(), outer.Field)
	require.NoError(t, err)
	require.Equal(t, map[string]string{}, observed)
}
