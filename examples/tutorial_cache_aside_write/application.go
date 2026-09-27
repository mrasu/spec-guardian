// Package cacheaside implements the cache-aside tutorial application.
package cacheaside

import (
	"context"
	"database/sql"

	"github.com/valkey-io/valkey-go"
)

// Application implements the cache-aside write action.
type Application struct {
	db    *postgresDB
	cache *valkeyCache
}

// NewApplication creates an application backed by the database and cache.
func NewApplication(database *sql.DB, cache valkey.Client) *Application {
	if database == nil {
		panic("cacheaside: nil database")
	}
	if cache == nil {
		panic("cacheaside: nil cache")
	}

	return &Application{
		db:    NewPostgresDB(database),
		cache: NewValkeyCache(cache),
	}
}

// WriteInput identifies the record and value to write.
type WriteInput struct {
	Key   string
	Value string
}

// WriteOutput contains the record written by Write.
type WriteOutput struct {
	Key   string
	Value string
}

// Write stores the value in the database and invalidates its cached entry.
func (a *Application) Write(ctx context.Context, input *WriteInput) (*WriteOutput, error) {
	if err := a.db.Write(ctx, input.Key, input.Value); err != nil {
		return nil, err
	}

	if err := a.cache.Delete(ctx, input.Key); err != nil {
		return nil, err
	}

	return &WriteOutput{Key: input.Key, Value: input.Value}, nil
}
