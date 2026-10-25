// Package sql implements the standard SQL field provider.
package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/provider"
	sqlfield "github.com/mrasu/spec-guardian/guardian/provider/sql/internal/field"
)

// Config supplies a SQL connection. Database can share an existing pool.
type Config struct {
	Database         *sql.DB
	Driver           string
	ConnectionString string
}

// Provider prepares and observes the SQL fields assigned to one provider.
type Provider struct {
	database   *sql.DB
	owned      bool
	fields     map[string]fieldObserver
	tables     map[string]*sqlfield.TableField
	fieldOrder []string
}

type fieldObserver interface {
	Observe(context.Context, *sql.DB) (any, error)
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.Seeder = (*Provider)(nil)

var _ provider.Observer = (*Provider)(nil)

// New validates the SQL requests and opens the provider's connection.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the SQL requests and opens the provider's connection.
// Register table fields in parent-before-child order for foreign key dependencies.
// Seed follows this order; CleanupAttempt clears in reverse order.
// Cyclic and self-referencing dependencies require separate preparation.
func NewProvider(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	settings, err := parseConfig(config, fields)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveConfig(dynamic, settings)
	if err != nil {
		return nil, err
	}
	observers, tables, err := validateFields(settings.Fields, settings.Tables, resolved.Driver)
	if err != nil {
		return nil, err
	}
	database, owned, err := openDatabase(resolved)
	if err != nil {
		return nil, err
	}
	order := make([]string, len(settings.Fields))
	for i, field := range settings.Fields {
		order[i] = field.Field
	}
	return &Provider{database: database, owned: owned, fields: observers, tables: tables, fieldOrder: order}, nil
}

func resolveConfig(dynamic Config, config *providerConfig) (Config, error) {
	if config.ConnectionString != "" {
		dynamic.ConnectionString = config.ConnectionString
	}
	if config.Driver != "" {
		dynamic.Driver = config.Driver
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

func validateFields(fields []sqlfield.Request, declarations []sqlfield.Table, driver string) (map[string]fieldObserver, map[string]*sqlfield.TableField, error) {
	tables, err := sqlfield.ResolveTables(declarations)
	if err != nil {
		return nil, nil, err
	}
	observers := make(map[string]fieldObserver, len(fields))
	tableObservers := make(map[string]*sqlfield.TableField)
	tableFields := make(map[string]string)
	for _, field := range fields {
		if field.Field == "" {
			return nil, nil, fmt.Errorf("SQL field ID is empty")
		}
		if _, exists := observers[field.Field]; exists {
			return nil, nil, fmt.Errorf("duplicate SQL field %q", field.Field)
		}
		var observer fieldObserver
		var err error
		if field.Config.Table != "" {
			var tableField *sqlfield.TableField
			tableField, err = sqlfield.NewTableField(field, tables, driver)
			if err == nil {
				name := tableField.TableName()
				observer = tableField
				tableObservers[field.Field] = tableField
				if previous, exists := tableFields[name]; exists {
					return nil, nil, fmt.Errorf("SQL table %q is assigned to both %q and %q; only one table field is supported", name, previous, field.Field)
				}
				tableFields[name] = field.Field
			}
		} else {
			observer, err = sqlfield.NewQueryField(field)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("SQL field %s: %w", field.Field, err)
		}
		observers[field.Field] = observer
	}
	return observers, tableObservers, nil
}

// PrepareCase begins a case.
func (o *Provider) PrepareCase(context.Context) error {
	return nil
}

// PrepareAttempt leaves existing data unchanged.
func (o *Provider) PrepareAttempt(context.Context) error { return nil }

// CleanupAttempt removes all rows from the assigned tables, including partial seeds.
func (o *Provider) CleanupAttempt(ctx context.Context) error {
	return o.clearTables(ctx)
}

// CleanupCase closes a connection owned by this provider.
func (o *Provider) CleanupCase(context.Context) error {
	if o.owned {
		return o.database.Close()
	}
	return nil
}

// Observe reads all rows for a field, preserving unexpected rows for comparison.
func (o *Provider) Observe(ctx context.Context, field string) (any, error) {
	observer, ok := o.fields[field]
	if !ok {
		return nil, fmt.Errorf("unknown SQL field %q", field)
	}
	result, err := observer.Observe(ctx, o.database)
	if err != nil {
		return nil, fmt.Errorf("SQL field %s: %w", field, err)
	}
	return result, nil
}

// Seed prepares table fields and passes observation-only query values through.
func (o *Provider) Seed(ctx context.Context, request provider.SeedRequest) (provider.SeedResult, error) {
	if len(request.Values) != len(o.fieldOrder) {
		return provider.SeedResult{}, fmt.Errorf("SQL seed requires all %d fields", len(o.fieldOrder))
	}
	indices := make(map[string]int, len(request.Values))
	for i, value := range request.Values {
		_, duplicate := indices[value.Field]
		if _, exists := o.fields[value.Field]; !exists || duplicate {
			return provider.SeedResult{}, fmt.Errorf("unknown or duplicate SQL seed field %q", value.Field)
		}
		indices[value.Field] = i
	}
	values := append([]provider.SeedValue(nil), request.Values...)
	for _, name := range o.fieldOrder {
		if table := o.tables[name]; table != nil {
			i := indices[name]
			value := values[i]
			prepared, err := table.Seed(ctx, o.database, value.Value)
			if err != nil {
				return provider.SeedResult{}, fmt.Errorf("SQL seed %s: %w", value.Field, err)
			}
			values[i].Value = prepared
		}
	}
	return provider.SeedResult{Values: values}, nil
}

func (o *Provider) clearTables(ctx context.Context) error {
	var failures []error
	for i := len(o.fieldOrder) - 1; i >= 0; i-- {
		name := o.fieldOrder[i]
		if table := o.tables[name]; table != nil {
			if err := table.Clear(ctx, o.database); err != nil {
				failures = append(failures, fmt.Errorf("SQL clear %s: %w", name, err))
			}
		}
	}
	return errors.Join(failures...)
}
