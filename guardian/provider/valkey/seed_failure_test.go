package valkey

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
	valkeygo "github.com/valkey-io/valkey-go"
)

// interruptedSeedClient cancels the second SET while forwarding cleanup commands.
type interruptedSeedClient struct {
	valkeygo.Client
	writes int
}

func NewInterruptedSeedClient(client valkeygo.Client) *interruptedSeedClient {
	if client == nil {
		panic("nil Valkey client")
	}
	return &interruptedSeedClient{Client: client}
}

func (c *interruptedSeedClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	if cmd.Commands()[0] == "SET" {
		c.writes++
		if c.writes == 2 {
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			return c.Client.Do(canceled, cmd)
		}
	}
	return c.Client.Do(ctx, cmd)
}

func TestProvider_SeedFailureReturnsZeroAndCleanupRemovesPartialState(t *testing.T) {
	client := newTestValkeyClient(t)
	prefix := fmt.Sprintf("sg-valkey-failure:%d:", time.Now().UnixNano())
	p, err := New(Config{Client: NewInterruptedSeedClient(client)}, map[string]any{"address": testValkeyAddress, "keyspaces": []any{testSpace(prefix)}}, []provider.FieldRequest{testKeyspaceField()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.CleanupAttempt(context.Background())) })
	result, err := p.Seed(t.Context(), provider.SeedRequest{Values: []provider.SeedValue{{Field: testField, Value: map[string]string{"a": "a", "b": "b"}}}})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, provider.SeedResult{}, result)
	partial, err := p.Observe(t.Context(), testField)
	require.NoError(t, err)
	require.Len(t, partial, 1)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	observed, err := p.Observe(t.Context(), testField)
	require.NoError(t, err)
	require.Equal(t, map[string]string{}, observed)
}
