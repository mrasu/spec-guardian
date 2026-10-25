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

// WriteAndInvalidateInput identifies the record and value to write.
type WriteAndInvalidateInput struct {
	Key   string
	Value string
}

// WriteAndInvalidateOutput contains the record written by WriteAndInvalidate.
type WriteAndInvalidateOutput struct {
	Key   string
	Value string
}

// WriteAndInvalidate stores the value in the database and invalidates its cached entry.
func (a *CacheAside) WriteAndInvalidate(ctx context.Context, input *WriteAndInvalidateInput) (*WriteAndInvalidateOutput, error) {
	if err := a.db.Write(ctx, input.Key, input.Value); err != nil {
		return nil, err
	}

	if err := a.cache.Delete(ctx, input.Key); err != nil {
		return nil, err
	}

	return &WriteAndInvalidateOutput{Key: input.Key, Value: input.Value}, nil
}
