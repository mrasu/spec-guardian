package cacheaside

import (
	"context"

	"github.com/valkey-io/valkey-go"
)

type valkeyCache struct {
	client valkey.Client
}

func NewValkeyCache(client valkey.Client) *valkeyCache {
	if client == nil {
		panic("cacheaside: nil cache")
	}
	return &valkeyCache{client: client}
}

func (cache *valkeyCache) Delete(ctx context.Context, key string) error {
	return cache.client.Do(ctx, cache.client.B().Del().Key(key).Build()).Error()
}
