// Package cacheaside implements the cache-aside example application.
package cacheaside

import (
	"context"
	"database/sql"

	"github.com/valkey-io/valkey-go"
)

// Application implements the cache-aside read and write actions.
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
		db:    &postgresDB{database: database},
		cache: &valkeyCache{client: cache},
	}
}

// ReadInput identifies the record to read.
type ReadInput struct {
	Key string
}

// ReadOutput contains the record found by Read, if any.
type ReadOutput struct {
	Key   string
	Value *string
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

// Read returns a cached value or loads it from the database.
func (a *Application) Read(ctx context.Context, input *ReadInput) (*ReadOutput, error) {
	value, found, err := a.cache.Get(ctx, input.Key)
	if err != nil {
		return nil, err
	}
	if found {
		return &ReadOutput{Key: input.Key, Value: &value}, nil
	}

	value, found, err = a.db.Read(ctx, input.Key)
	if err != nil {
		return nil, err
	}
	if !found {
		return &ReadOutput{Key: input.Key}, nil
	}

	if err := a.cache.Set(ctx, input.Key, value); err != nil {
		return nil, err
	}
	return &ReadOutput{Key: input.Key, Value: &value}, nil
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
