// Package dynamodb prepares and observes records stored in DynamoDB tables.
package dynamodb

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsv2dynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mrasu/spec-guardian/guardian/provider"
	dynamofield "github.com/mrasu/spec-guardian/guardian/provider/dynamodb/internal/field"
)

// Config supplies an existing client when one is available.
type Config struct {
	Client *awsv2dynamodb.Client
}

// Provider prepares and reads the DynamoDB fields assigned to one conformance case.
type Provider struct {
	client *awsv2dynamodb.Client
	fields map[string]*dynamofield.TableField
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.Seeder = (*Provider)(nil)

var _ provider.Observer = (*Provider)(nil)

// New validates the requests and prepares a DynamoDB client.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the requests and prepares a DynamoDB client.
func NewProvider(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	settings, err := parseConfig(config, fields)
	if err != nil {
		return nil, err
	}
	tables, err := dynamofield.ResolveTables(settings.Tables)
	if err != nil {
		return nil, err
	}
	resolved, err := validateFields(settings.Fields, tables)
	if err != nil {
		return nil, err
	}
	client, err := newClient(dynamic, settings)
	if err != nil {
		return nil, err
	}
	return &Provider{client: client, fields: resolved}, nil
}

func newClient(dynamic Config, config *providerConfig) (*awsv2dynamodb.Client, error) {
	client := dynamic.Client
	if client != nil {
		return client, nil
	}
	for _, setting := range []struct{ name, value string }{
		{"endpoint", config.Endpoint},
		{"region", config.Region},
		{"access_key_id", config.AccessKeyID},
		{"secret_access_key", config.SecretAccessKey},
	} {
		if setting.value == "" {
			return nil, fmt.Errorf("%s is required when no DynamoDB client is supplied", setting.name)
		}
	}
	return awsv2dynamodb.New(awsv2dynamodb.Options{
		BaseEndpoint: aws.String(config.Endpoint),
		Region:       config.Region,
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")),
		Retryer:      aws.NopRetryer{},
	}), nil
}

func validateFields(fields []dynamofield.Request, declarations map[string]dynamofield.Table) (map[string]*dynamofield.TableField, error) {
	resolved := make(map[string]*dynamofield.TableField, len(fields))
	tables := make(map[string]string, len(fields))
	for _, request := range fields {
		if request.Field == "" {
			return nil, fmt.Errorf("DynamoDB field is empty")
		}
		if _, exists := resolved[request.Field]; exists {
			return nil, fmt.Errorf("duplicate DynamoDB field %q", request.Field)
		}
		table, err := dynamofield.NewTableField(request, declarations)
		if err != nil {
			return nil, err
		}
		if previous, exists := tables[table.TableName()]; exists {
			return nil, fmt.Errorf("DynamoDB fields %s and %s reference the same table %q", previous, request.Field, table.TableName())
		}
		tables[table.TableName()] = request.Field
		resolved[request.Field] = table
	}
	return resolved, nil
}

// PrepareCase begins a case.
func (o *Provider) PrepareCase(context.Context) error { return nil }

// PrepareAttempt leaves existing data unchanged.
func (o *Provider) PrepareAttempt(context.Context) error { return nil }

// CleanupAttempt removes unexpected items and partially seeded data.
func (o *Provider) CleanupAttempt(ctx context.Context) error { return o.clearTables(ctx) }

// CleanupCase ends a case.
func (o *Provider) CleanupCase(context.Context) error { return nil }

// Observe scans every page and retains unexpected records for comparison.
func (o *Provider) Observe(ctx context.Context, field string) (any, error) {
	table, ok := o.fields[field]
	if !ok {
		return nil, fmt.Errorf("unknown DynamoDB field %q", field)
	}
	return table.Observe(ctx, o.client)
}

// Seed writes the model's values without changing field or list order.
func (o *Provider) Seed(ctx context.Context, request provider.SeedRequest) (provider.SeedResult, error) {
	items, err := o.buildSeedItems(request)
	if err != nil {
		return provider.SeedResult{}, err
	}
	for i, value := range request.Values {
		if err := o.fields[value.Field].Seed(ctx, o.client, items[i]); err != nil {
			return provider.SeedResult{}, fmt.Errorf("DynamoDB seed %s: %w", value.Field, err)
		}
	}

	return provider.SeedResult{Values: append([]provider.SeedValue(nil), request.Values...)}, nil
}

func (o *Provider) buildSeedItems(request provider.SeedRequest) ([][]map[string]types.AttributeValue, error) {
	if len(request.Values) != len(o.fields) {
		return nil, fmt.Errorf("DynamoDB seed requires all %d fields", len(o.fields))
	}
	items := make([][]map[string]types.AttributeValue, len(request.Values))
	seen := make(map[string]bool, len(request.Values))
	for i, value := range request.Values {
		field, exists := o.fields[value.Field]
		if !exists || seen[value.Field] {
			return nil, fmt.Errorf("unknown or duplicate DynamoDB seed field %q", value.Field)
		}
		seen[value.Field] = true
		records, err := field.ToSeedItems(value.Value)
		if err != nil {
			return nil, fmt.Errorf("DynamoDB seed %s: %w", value.Field, err)
		}
		items[i] = records
	}
	return items, nil
}

func (o *Provider) clearTables(ctx context.Context) error {
	var failures []error
	for field, table := range o.fields {
		if err := table.Clear(ctx, o.client); err != nil {
			failures = append(failures, fmt.Errorf("DynamoDB clear %s: %w", field, err))
		}
	}
	return errors.Join(failures...)
}
