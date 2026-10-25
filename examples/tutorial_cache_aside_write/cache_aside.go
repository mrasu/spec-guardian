// Package cacheaside implements the cache-aside tutorial application.
package cacheaside

import (
	"context"
	"database/sql"

	"github.com/valkey-io/valkey-go"
)

// CacheAside implements the cache-aside write action.
type CacheAside struct {
	db    *postgresDB
	cache *valkeyCache
}

// NewCacheAside creates an application backed by the database and cache.
func NewCacheAside(database *sql.DB, cache valkey.Client) *CacheAside {
	if database == nil {
		panic("cacheaside: nil database")
	}
	if cache == nil {
		panic("cacheaside: nil cache")
	}

	return &CacheAside{
		db:    NewPostgresDB(database),
		cache: NewValkeyCache(cache),
	}
}

// Input identifies the record and value to write.
type Input struct {
	Key   string
	Value string
}

// Output contains the record written by WriteAndInvalidate.
type Output struct {
	Key   string
	Value string
}

// WriteAndInvalidate stores the value in the database and invalidates its cached entry.
func (ca *CacheAside) WriteAndInvalidate(ctx context.Context, input *Input) (*Output, error) {
	if err := ca.db.Write(ctx, input.Key, input.Value); err != nil {
		return nil, err
	}

	if err := ca.cache.Delete(ctx, input.Key); err != nil {
		return nil, err
	}

	return &Output{Key: input.Key, Value: input.Value}, nil
}
