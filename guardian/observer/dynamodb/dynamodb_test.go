package dynamodb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mrasu/spec-guardian/guardian/observer"
	"github.com/stretchr/testify/require"
)

func TestToRecord_ConvertsNullableInt64(t *testing.T) {
	fields := map[string]observer.Type{
		"order_id": {Kind: observer.KindInt64, Nullable: true},
		"status":   {Kind: observer.KindString},
	}
	tests := []struct {
		name         string
		keyAttribute string
		item         map[string]types.AttributeValue
		want         map[string]any
		err          string
	}{
		{
			name:         "integer string",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "order_id": &types.AttributeValueMemberS{Value: "42"}, "status": &types.AttributeValueMemberS{Value: "DONE"}},
			want:         map[string]any{"order_id": int64(42), "status": "DONE"},
		},
		{
			name:         "absent nullable integer",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "status": &types.AttributeValueMemberS{Value: "INIT"}},
			want:         map[string]any{"order_id": nil, "status": "INIT"},
		},
		{
			name:         "invalid integer",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "order_id": &types.AttributeValueMemberS{Value: "1.5"}, "status": &types.AttributeValueMemberS{Value: "DONE"}},
			err:          "invalid int64",
		},
		{
			name:         "extra attribute",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "status": &types.AttributeValueMemberS{Value: "INIT"}, "extra": &types.AttributeValueMemberS{Value: "x"}},
			err:          "extra attribute",
		},
		{
			name:         "missing required attribute",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}},
			err:          "missing attribute",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, got, err := toRecord(tt.item, tt.keyAttribute, fields)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Empty(t, key)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "request-1", key)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestToRecord_UsesConfiguredKeyAttribute(t *testing.T) {
	fields := map[string]observer.Type{
		"id":     {Kind: observer.KindString},
		"status": {Kind: observer.KindString},
	}
	item := map[string]types.AttributeValue{
		"request_id": &types.AttributeValueMemberS{Value: "request-1"},
		"id":         &types.AttributeValueMemberS{Value: "record-1"},
		"status":     &types.AttributeValueMemberS{Value: "DONE"},
	}

	key, record, err := toRecord(item, "request_id", fields)

	require.NoError(t, err)
	require.Equal(t, "request-1", key)
	require.Equal(t, map[string]any{"id": "record-1", "status": "DONE"}, record)
}

func TestNewObserver_RejectsInvalidFieldRequest(t *testing.T) {
	valid := observer.FieldRequest{
		Field:  "DynamoDB.order_idempotency_records",
		Config: map[string]any{"table": "order_idempotency_records", "result": map[string]any{"type": "item_map", "key_attribute": "id"}},
		Output: observer.OutputSpec{Type: observer.Type{
			Kind: observer.KindMap,
			Values: &observer.Type{
				Kind: observer.KindMessage,
				Fields: map[string]observer.Type{
					"order_id": {Kind: observer.KindInt64, Nullable: true},
				},
			},
		}},
	}
	tests := []struct {
		name  string
		field observer.FieldRequest
		want  string
	}{
		{name: "missing table", field: observer.FieldRequest{Field: valid.Field, Output: valid.Output}, want: "table must be a nonempty string"},
		{name: "missing result", field: observer.FieldRequest{Field: valid.Field, Config: map[string]any{"table": "order_idempotency_records"}, Output: valid.Output}, want: "result.type must be item_map"},
		{name: "missing key attribute", field: observer.FieldRequest{Field: valid.Field, Config: map[string]any{"table": "order_idempotency_records", "result": map[string]any{"type": "item_map"}}, Output: valid.Output}, want: "result.key_attribute must be a nonempty string"},
		{name: "wrong output", field: observer.FieldRequest{Field: valid.Field, Config: valid.Config, Output: observer.OutputSpec{Type: observer.Type{Kind: observer.KindString}}}, want: "requires a non-null map of messages"},
		{name: "key inside message", field: observer.FieldRequest{Field: valid.Field, Config: valid.Config, Output: observer.OutputSpec{Type: observer.Type{Kind: observer.KindMap, Values: &observer.Type{Kind: observer.KindMessage, Fields: map[string]observer.Type{"id": {Kind: observer.KindString}}}}}}, want: "key attribute \"id\" cannot also be a message field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewObserver(Config{}, nil, []observer.FieldRequest{tt.field})
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, got)
		})
	}
}
