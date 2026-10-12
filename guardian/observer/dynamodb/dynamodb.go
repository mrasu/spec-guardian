// Package dynamodb observes records stored in DynamoDB tables.
package dynamodb

import (
	"context"
	"fmt"
	"maps"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsv2dynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mrasu/spec-guardian/guardian/observer"
)

// Config supplies an existing client when one is available.
type Config struct {
	Client *awsv2dynamodb.Client
}

// Observer reads the DynamoDB fields assigned to one conformance case.
type Observer struct {
	client   *awsv2dynamodb.Client
	requests map[string]observer.FieldRequest
}

var _ observer.Observer = (*Observer)(nil)

// New validates the requests and prepares a DynamoDB client.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the requests and prepares a DynamoDB client.
func NewObserver(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	requests, err := validateFields(fields)
	if err != nil {
		return nil, err
	}
	client, err := newClient(dynamic, config)
	if err != nil {
		return nil, err
	}
	return &Observer{client: client, requests: requests}, nil
}

func newClient(dynamic Config, config map[string]any) (*awsv2dynamodb.Client, error) {
	client := dynamic.Client
	if client != nil {
		return client, nil
	}
	endpoint, err := requiredString(config, "endpoint")
	if err != nil {
		return nil, err
	}
	region, err := requiredString(config, "region")
	if err != nil {
		return nil, err
	}
	accessKey, err := requiredString(config, "access_key_id")
	if err != nil {
		return nil, err
	}
	secretKey, err := requiredString(config, "secret_access_key")
	if err != nil {
		return nil, err
	}
	return awsv2dynamodb.New(awsv2dynamodb.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       region,
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		Retryer:      aws.NopRetryer{},
	}), nil
}

func validateFields(fields []observer.FieldRequest) (map[string]observer.FieldRequest, error) {
	requests := make(map[string]observer.FieldRequest, len(fields))
	for _, field := range fields {
		if field.Field == "" {
			return nil, fmt.Errorf("DynamoDB field is empty")
		}
		if _, exists := requests[field.Field]; exists {
			return nil, fmt.Errorf("duplicate DynamoDB field %q", field.Field)
		}
		_, err := requiredString(field.Config, "table")
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Field, err)
		}
		result, ok := field.Config["result"].(map[string]any)
		if !ok || result["type"] != "item_map" {
			return nil, fmt.Errorf("field %s: result.type must be item_map", field.Field)
		}
		keyAttribute, err := requiredString(result, "key_attribute")
		if err != nil {
			return nil, fmt.Errorf("field %s: result.%w", field.Field, err)
		}
		spec := field.Output.Type
		if spec.Kind != observer.KindMap || spec.Nullable || spec.Values == nil || spec.Values.Kind != observer.KindMessage || spec.Values.Nullable || len(spec.Values.Fields) == 0 {
			return nil, fmt.Errorf("field %s requires a non-null map of messages", field.Field)
		}
		if _, exists := spec.Values.Fields[keyAttribute]; exists {
			return nil, fmt.Errorf("field %s: key attribute %q cannot also be a message field", field.Field, keyAttribute)
		}
		for name, spec := range spec.Values.Fields {
			if name == "" || spec.Values != nil || len(spec.Fields) != 0 || spec.Kind != observer.KindString && spec.Kind != observer.KindInt64 {
				return nil, fmt.Errorf("field %s has unsupported message field %q", field.Field, name)
			}
		}
		field.Config = maps.Clone(field.Config)
		field.Config["result"] = maps.Clone(result)
		values := *field.Output.Type.Values
		values.Fields = maps.Clone(values.Fields)
		field.Output.Type.Values = &values
		requests[field.Field] = field
	}
	return requests, nil
}

func requiredString(config map[string]any, key string) (string, error) {
	value, ok := config[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a nonempty string", key)
	}
	return value, nil
}

// PrepareCase begins a case.
func (o *Observer) PrepareCase(context.Context) error { return nil }

// PrepareAttempt begins an attempt.
func (o *Observer) PrepareAttempt(context.Context) error { return nil }

// CleanupAttempt ends an attempt.
func (o *Observer) CleanupAttempt(context.Context) error { return nil }

// CleanupCase ends a case.
func (o *Observer) CleanupCase(context.Context) error { return nil }

// Observe scans every page and retains unexpected records for comparison.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	request, ok := o.requests[field]
	if !ok {
		return nil, fmt.Errorf("unknown DynamoDB field %q", field)
	}
	table := request.Config["table"].(string)
	keyAttribute := request.Config["result"].(map[string]any)["key_attribute"].(string)
	records, err := o.scanRecords(ctx, table, keyAttribute, request.Output.Type.Values.Fields)
	if err != nil {
		return nil, fmt.Errorf("table %s: %w", table, err)
	}
	return records, nil
}

func (o *Observer) scanRecords(ctx context.Context, table, keyAttribute string, fields map[string]observer.Type) (map[string]any, error) {
	records := make(map[string]any)
	var cursor map[string]types.AttributeValue
	for {
		page, err := o.client.Scan(ctx, &awsv2dynamodb.ScanInput{
			TableName:         aws.String(table),
			ExclusiveStartKey: cursor,
			ConsistentRead:    aws.Bool(true),
		})
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if err := addRecords(records, page.Items, keyAttribute, fields); err != nil {
			return nil, err
		}
		if len(page.LastEvaluatedKey) == 0 {
			return records, nil
		}
		cursor = page.LastEvaluatedKey
	}
}

func addRecords(records map[string]any, items []map[string]types.AttributeValue, keyAttribute string, fields map[string]observer.Type) error {
	for _, item := range items {
		key, record, err := toRecord(item, keyAttribute, fields)
		if err != nil {
			return err
		}
		if _, exists := records[key]; exists {
			return fmt.Errorf("duplicate key %q", key)
		}
		records[key] = record
	}
	return nil
}

func toRecord(item map[string]types.AttributeValue, keyAttribute string, fields map[string]observer.Type) (string, map[string]any, error) {
	keyValue, ok := item[keyAttribute].(*types.AttributeValueMemberS)
	if !ok {
		return "", nil, fmt.Errorf("key attribute %q is missing or is not a string", keyAttribute)
	}
	record := make(map[string]any, len(fields))
	for name, spec := range fields {
		attribute, exists := item[name]
		if !exists {
			if !spec.Nullable {
				return "", nil, fmt.Errorf("record %q: missing attribute %q", keyValue.Value, name)
			}
			record[name] = nil
			continue
		}
		value, err := decodeAttribute(attribute, spec)
		if err != nil {
			return "", nil, fmt.Errorf("record %q attribute %q: %w", keyValue.Value, name, err)
		}
		record[name] = value
	}
	for name := range item {
		if name != keyAttribute {
			if _, known := fields[name]; !known {
				return "", nil, fmt.Errorf("record %q: extra attribute %q", keyValue.Value, name)
			}
		}
	}
	return keyValue.Value, record, nil
}

func decodeAttribute(attribute types.AttributeValue, spec observer.Type) (any, error) {
	if _, ok := attribute.(*types.AttributeValueMemberNULL); ok {
		if spec.Nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("null is not allowed")
	}
	text, ok := attribute.(*types.AttributeValueMemberS)
	if !ok {
		return nil, fmt.Errorf("got %T, want string attribute", attribute)
	}
	if spec.Kind == observer.KindString {
		return text.Value, nil
	}
	number, err := strconv.ParseInt(text.Value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid int64 %q: %w", text.Value, err)
	}
	return number, nil
}
