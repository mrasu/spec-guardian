package cacheaside

import (
	"context"

	"github.com/valkey-io/valkey-go"
)

type valkeyCache struct {
	client valkey.Client
}

func (cache *valkeyCache) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := cache.client.Do(ctx, cache.client.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (cache *valkeyCache) Set(ctx context.Context, key string, value string) error {
	return cache.client.Do(ctx, cache.client.B().Set().Key(key).Value(value).Build()).Error()
}

func (cache *valkeyCache) Delete(ctx context.Context, key string) error {
	return cache.client.Do(ctx, cache.client.B().Del().Key(key).Build()).Error()
}
