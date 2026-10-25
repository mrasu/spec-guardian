package field

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestToRecord_ConvertsNullableInt64(t *testing.T) {
	fields := map[string]provider.Type{
		"order_id": {Kind: provider.KindInt64, Nullable: true},
		"status":   {Kind: provider.KindString},
	}
	tests := []struct {
		name         string
		keyAttribute string
		item         map[string]types.AttributeValue
		want         map[string]any
		err          string
	}{
		{
			name:         "number attribute",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "order_id": &types.AttributeValueMemberN{Value: "42"}, "status": &types.AttributeValueMemberS{Value: "DONE"}},
			want:         map[string]any{"order_id": int64(42), "status": "DONE"},
		},
		{
			name:         "explicit null integer",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "status": &types.AttributeValueMemberS{Value: "INIT"}, "order_id": &types.AttributeValueMemberNULL{Value: true}},
			want:         map[string]any{"order_id": nil, "status": "INIT"},
		},
		{
			name:         "invalid integer",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "order_id": &types.AttributeValueMemberN{Value: "1.5"}, "status": &types.AttributeValueMemberS{Value: "DONE"}},
			err:          "invalid int64",
		},
		{
			name:         "extra attribute",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "status": &types.AttributeValueMemberS{Value: "INIT"}, "order_id": &types.AttributeValueMemberNULL{Value: true}, "extra": &types.AttributeValueMemberS{Value: "x"}},
			err:          "extra attribute",
		},
		{
			name:         "missing required attribute",
			keyAttribute: "id",
			item:         map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "request-1"}, "order_id": &types.AttributeValueMemberNULL{Value: true}},
			err:          `missing attribute "status"`,
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
	fields := map[string]provider.Type{
		"id":     {Kind: provider.KindString},
		"status": {Kind: provider.KindString},
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

func TestSeedAttribute_RoundTripsModelTypesAndExplicitNull(t *testing.T) {
	tests := []struct {
		name      string
		spec      provider.Type
		value     any
		attribute types.AttributeValue
	}{
		{name: "string", spec: provider.Type{Kind: provider.KindString}, value: "42", attribute: &types.AttributeValueMemberS{Value: "42"}},
		{name: "integer", spec: provider.Type{Kind: provider.KindInt64}, value: int64(-42), attribute: &types.AttributeValueMemberN{Value: "-42"}},
		{name: "boolean", spec: provider.Type{Kind: provider.KindBool}, value: false, attribute: &types.AttributeValueMemberBOOL{Value: false}},
		{name: "null string", spec: provider.Type{Kind: provider.KindString, Nullable: true}, value: nil, attribute: &types.AttributeValueMemberNULL{Value: true}},
		{name: "null integer", spec: provider.Type{Kind: provider.KindInt64, Nullable: true}, value: nil, attribute: &types.AttributeValueMemberNULL{Value: true}},
		{name: "null boolean", spec: provider.Type{Kind: provider.KindBool, Nullable: true}, value: nil, attribute: &types.AttributeValueMemberNULL{Value: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attribute, err := toSeedAttribute(tt.value, tt.spec)
			require.NoError(t, err)
			require.Equal(t, tt.attribute, attribute)
			value, err := toModelValue(attribute, tt.spec)
			require.NoError(t, err)
			require.Equal(t, tt.value, value)
		})
	}
}

func TestToRecord_RejectsMissingNullableAttributeAndWrongStoredTypes(t *testing.T) {
	tests := []struct {
		name      string
		attribute types.AttributeValue
		spec      provider.Type
		want      string
	}{
		{name: "absent nullable attribute", spec: provider.Type{Kind: provider.KindString, Nullable: true}, want: "missing attribute"},
		{name: "string stored for integer", attribute: &types.AttributeValueMemberS{Value: "42"}, spec: provider.Type{Kind: provider.KindInt64}, want: "model type int64"},
		{name: "number stored for string", attribute: &types.AttributeValueMemberN{Value: "42"}, spec: provider.Type{Kind: provider.KindString}, want: "model type string"},
		{name: "null in required attribute", attribute: &types.AttributeValueMemberNULL{Value: true}, spec: provider.Type{Kind: provider.KindString}, want: "NULL requires"},
		{name: "false null marker", attribute: &types.AttributeValueMemberNULL{Value: false}, spec: provider.Type{Kind: provider.KindString, Nullable: true}, want: "NULL requires"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "key"}}
			if tt.attribute != nil {
				item["value"] = tt.attribute
			}
			key, row, err := toRecord(item, "id", map[string]provider.Type{"value": tt.spec})
			require.ErrorContains(t, err, tt.want)
			require.Empty(t, key)
			require.Nil(t, row)
		})
	}
}
