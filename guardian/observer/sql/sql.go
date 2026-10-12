// Package sql implements the standard SQL field observer.
package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/observer"
	sqlfield "github.com/mrasu/spec-guardian/guardian/observer/sql/internal/field"
)

// Config supplies a SQL connection. Database can share an existing pool.
type Config struct {
	Database         *sql.DB
	Driver           string
	ConnectionString string
}

// Observer reads the SQL fields assigned to one observer.
type Observer struct {
	database *sql.DB
	owned    bool
	fields   map[string]*sqlfield.Query
}

var _ observer.Observer = (*Observer)(nil)

// New validates the SQL requests and opens the observer's connection.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the SQL requests and opens the observer's connection.
func NewObserver(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	resolved, err := resolveConfig(dynamic, config)
	if err != nil {
		return nil, err
	}
	queries, err := validateFields(fields)
	if err != nil {
		return nil, err
	}
	database, owned, err := openDatabase(resolved)
	if err != nil {
		return nil, err
	}
	return &Observer{database: database, owned: owned, fields: queries}, nil
}

func resolveConfig(dynamic Config, config map[string]any) (Config, error) {
	if raw, exists := config["connection_string"]; exists {
		connection, ok := raw.(string)
		if !ok || connection == "" {
			return Config{}, fmt.Errorf("SQL connection_string must be a nonempty string")
		}
		dynamic.ConnectionString = connection
	}
	if raw, exists := config["driver"]; exists {
		driver, ok := raw.(string)
		if !ok || driver == "" {
			return Config{}, fmt.Errorf("SQL driver must be a nonempty string")
		}
		dynamic.Driver = driver
	}
	if dynamic.Driver == "" {
		return Config{}, fmt.Errorf("SQL driver is required")
	}
	return dynamic, nil
}

func openDatabase(config Config) (*sql.DB, bool, error) {
	database := config.Database
	owned := false
	if database == nil {
		if config.ConnectionString == "" {
			return nil, false, fmt.Errorf("SQL connection_string is required")
		}
		var err error
		database, err = sql.Open(config.Driver, config.ConnectionString)
		if err != nil {
			return nil, false, fmt.Errorf("open SQL connection: %w", err)
		}
		owned = true
	}
	return database, owned, nil
}

func validateFields(fields []observer.FieldRequest) (map[string]*sqlfield.Query, error) {
	queries := make(map[string]*sqlfield.Query, len(fields))
	for _, field := range fields {
		if field.Field == "" {
			return nil, fmt.Errorf("SQL field ID is empty")
		}
		if _, exists := queries[field.Field]; exists {
			return nil, fmt.Errorf("duplicate SQL field %q", field.Field)
		}
		query, err := sqlfield.NewQuery(field)
		if err != nil {
			return nil, err
		}
		queries[field.Field] = query
	}
	return queries, nil
}

// PrepareCase begins a case.
func (o *Observer) PrepareCase(context.Context) error {
	return nil
}

// PrepareAttempt begins an attempt.
func (o *Observer) PrepareAttempt(context.Context) error {
	return nil
}

// CleanupAttempt ends an attempt.
func (o *Observer) CleanupAttempt(context.Context) error {
	return nil
}

// CleanupCase closes a connection owned by this observer.
func (o *Observer) CleanupCase(context.Context) error {
	if o.owned {
		return o.database.Close()
	}
	return nil
}

// Observe reads all rows for a field, preserving unexpected rows for comparison.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	query, ok := o.fields[field]
	if !ok {
		return nil, fmt.Errorf("unknown SQL field %q", field)
	}
	rows, err := o.database.QueryContext(ctx, query.SQL())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := query.Read(rows)
	if err != nil {
		return nil, fmt.Errorf("SQL field %s: %w", field, err)
	}
	return result, nil
}
