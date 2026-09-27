package gvalkeygo

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeyhook"
)

const valkeyAddress = "localhost:16379"

func TestValkeyIntegration(t *testing.T) {
	t.Run("success path", func(t *testing.T) {
		fault := &rejectingFault{}
		client := openValkeyClient(t, fault)

		require.NoError(t, client.Do(t.Context(), client.B().Ping().Build()).Error())
		assert.Equal(t, []string{"valkey.Do"}, fault.operations)
	})

	t.Run("fault", func(t *testing.T) {
		injected := errors.New("injected")
		fault := &rejectingFault{err: injected}
		client := openValkeyClient(t, fault)

		result := client.Do(t.Context(), client.B().Ping().Build())

		assert.ErrorIs(t, result.Error(), injected)
		assert.Equal(t, []string{"valkey.Do"}, fault.operations)
	})
}

func openValkeyClient(t *testing.T, fault *rejectingFault) valkey.Client {
	t.Helper()
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddress}})
	require.NoError(t, err)
	client = valkeyhook.WithHook(client, NewHook(fault))
	t.Cleanup(client.Close)
	return client
}
