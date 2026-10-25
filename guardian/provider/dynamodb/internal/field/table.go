// Package field handles DynamoDB table fields and their model representation.
package field

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsv2dynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mrasu/spec-guardian/guardian/provider"
)

// TableField prepares and observes a table with one string key attribute.
type TableField struct {
	table        string
	keyAttribute string
	resultType   ResultType
	fields       map[string]provider.Type
}

// NewTableField resolves schema-validated settings against the model type and tables.
func NewTableField(request Request, tables map[string]Table) (*TableField, error) {
	table := request.Config.Table
	resultType := request.Config.Result.Type
	declaration, exists := tables[table]
	if !exists {
		return nil, fmt.Errorf("field %s: unknown DynamoDB table %q", request.Field, table)
	}
	keyAttribute := declaration.KeySchema[0].AttributeName
	spec := request.Output.Type
	if (spec.Kind != provider.KindMap && spec.Kind != provider.KindList) || spec.Nullable || spec.Values == nil || spec.Values.Kind != provider.KindMessage || spec.Values.Nullable || len(spec.Values.Fields) == 0 {
		return nil, fmt.Errorf("field %s requires a non-null map or list of messages", request.Field)
	}
	if _, exists := spec.Values.Fields[keyAttribute]; exists && resultType == ResultTypeItemMap {
		return nil, fmt.Errorf("field %s: key attribute %q cannot also be a message field", request.Field, keyAttribute)
	}
	if resultType == ResultTypeItemList {
		key, exists := spec.Values.Fields[keyAttribute]
		if spec.Kind != provider.KindList || !exists || key.Kind != provider.KindString || key.Nullable {
			return nil, fmt.Errorf("field %s: item_list requires a list with non-null string key field %q", request.Field, keyAttribute)
		}
	} else if spec.Kind != provider.KindMap {
		return nil, fmt.Errorf("field %s: item_map requires a map of messages", request.Field)
	}
	for name, spec := range spec.Values.Fields {
		if name == "" || spec.Values != nil || len(spec.Fields) != 0 || spec.Kind != provider.KindString && spec.Kind != provider.KindInt64 && spec.Kind != provider.KindBool {
			return nil, fmt.Errorf("field %s has unsupported message field %q", request.Field, name)
		}
	}
	return &TableField{table: table, keyAttribute: keyAttribute, resultType: resultType, fields: maps.Clone(spec.Values.Fields)}, nil
}

// TableName returns the table assigned to this field.
func (f *TableField) TableName() string { return f.table }

// Observe returns every row, retaining unexpected records for comparison.
func (f *TableField) Observe(ctx context.Context, client *awsv2dynamodb.Client) (any, error) {
	records, err := f.scanRecords(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("table %s: %w", f.table, err)
	}
	if f.resultType == ResultTypeItemList {
		rows := make([]map[string]any, 0, len(records))
		for _, record := range records {
			rows = append(rows, record)
		}
		return rows, nil
	}
	return records, nil
}

func (f *TableField) scanRecords(ctx context.Context, client *awsv2dynamodb.Client) (map[string]map[string]any, error) {
	records := make(map[string]map[string]any)
	var cursor map[string]types.AttributeValue
	for {
		page, err := client.Scan(ctx, &awsv2dynamodb.ScanInput{
			TableName:         aws.String(f.table),
			ExclusiveStartKey: cursor,
			ConsistentRead:    aws.Bool(true),
		})
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if err := addRecords(records, page.Items, f.keyAttribute, f.fields); err != nil {
			return nil, err
		}
		if len(page.LastEvaluatedKey) == 0 {
			return records, nil
		}
		cursor = page.LastEvaluatedKey
	}
}

// ToSeedItems validates and converts a model value before any table is written.
func (f *TableField) ToSeedItems(value any) ([]map[string]types.AttributeValue, error) {
	return toSeedItems(value, f.resultType, f.keyAttribute, f.fields)
}

// Seed inserts validated items in their supplied order.
func (f *TableField) Seed(ctx context.Context, client *awsv2dynamodb.Client, items []map[string]types.AttributeValue) error {
	for _, item := range items {
		if _, err := client.PutItem(ctx, &awsv2dynamodb.PutItemInput{TableName: aws.String(f.table), Item: item}); err != nil {
			return err
		}
	}
	return nil
}

// Clear removes all items, including partial seeds and malformed values.
func (f *TableField) Clear(ctx context.Context, client *awsv2dynamodb.Client) error {
	keys, err := f.scanKeys(ctx, client)
	if err != nil {
		return err
	}
	var failures []error
	for _, key := range keys {
		if _, err := client.DeleteItem(ctx, &awsv2dynamodb.DeleteItemInput{TableName: aws.String(f.table), Key: key}); err != nil {
			failures = append(failures, fmt.Errorf("delete: %w", err))
		}
	}
	return errors.Join(failures...)
}

func (f *TableField) scanKeys(ctx context.Context, client *awsv2dynamodb.Client) ([]map[string]types.AttributeValue, error) {
	var keys []map[string]types.AttributeValue
	var cursor map[string]types.AttributeValue
	// Finish scanning before deleting so pagination never depends on deleted items.
	for {
		page, err := client.Scan(ctx, &awsv2dynamodb.ScanInput{
			TableName: aws.String(f.table), ExclusiveStartKey: cursor, ConsistentRead: aws.Bool(true),
			ProjectionExpression: aws.String("#key"), ExpressionAttributeNames: map[string]string{"#key": f.keyAttribute},
		})
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		for _, item := range page.Items {
			key, ok := item[f.keyAttribute]
			if !ok {
				return nil, fmt.Errorf("missing key attribute %q", f.keyAttribute)
			}
			keys = append(keys, map[string]types.AttributeValue{f.keyAttribute: key})
		}
		if len(page.LastEvaluatedKey) == 0 {
			return keys, nil
		}
		cursor = page.LastEvaluatedKey
	}
}
