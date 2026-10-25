package field

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestNewTableField_RejectsInvalidFieldRequest(t *testing.T) {
	valid := Request{
		Field:  "DynamoDB.order_idempotency_records",
		Config: Config{Table: "order_idempotency_records", Result: ResultConfig{Type: ResultTypeItemMap}},
		Output: provider.OutputSpec{Type: provider.Type{
			Kind: provider.KindMap,
			Values: &provider.Type{
				Kind: provider.KindMessage,
				Fields: map[string]provider.Type{
					"order_id": {Kind: provider.KindInt64, Nullable: true},
				},
			},
		}},
	}
	tests := []struct {
		name  string
		field Request
		want  string
	}{
		{name: "unknown table", field: Request{Field: valid.Field, Config: Config{Table: "unknown", Result: ResultConfig{Type: ResultTypeItemMap}}, Output: valid.Output}, want: "unknown DynamoDB table"},
		{name: "wrong output", field: Request{Field: valid.Field, Config: valid.Config, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindString}}}, want: "requires a non-null map or list of messages"},
		{name: "key inside message", field: Request{Field: valid.Field, Config: valid.Config, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"id": {Kind: provider.KindString}}}}}}, want: "key attribute \"id\" cannot also be a message field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTableField(tt.field, testTables("order_idempotency_records"))
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, got)
		})
	}
}

func TestNewTableField_ItemListRequiresStringKeyInRows(t *testing.T) {
	tests := []struct {
		name   string
		kind   provider.Kind
		fields map[string]provider.Type
	}{
		{name: "missing key", kind: provider.KindList, fields: map[string]provider.Type{"data": {Kind: provider.KindString}}},
		{name: "nullable key", kind: provider.KindList, fields: map[string]provider.Type{"id": {Kind: provider.KindString, Nullable: true}}},
		{name: "integer key", kind: provider.KindList, fields: map[string]provider.Type{"id": {Kind: provider.KindInt64}}},
		{name: "map output", kind: provider.KindMap, fields: map[string]provider.Type{"id": {Kind: provider.KindString}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTableField(Request{
				Field:  "deliveries",
				Config: Config{Table: "deliveries", Result: ResultConfig{Type: ResultTypeItemList}},
				Output: provider.OutputSpec{Type: provider.Type{Kind: tt.kind, Values: &provider.Type{Kind: provider.KindMessage, Fields: tt.fields}}},
			}, testTables("deliveries"))
			require.ErrorContains(t, err, "item_list requires a list with non-null string key field")
			require.Nil(t, got)
		})
	}
}

func testTables(names ...string) map[string]Table {
	tables := make(map[string]Table, len(names))
	for _, name := range names {
		tables[name] = Table{Name: name,
			AttributeDefinitions: []AttributeDefinition{{AttributeName: "id", AttributeType: AttributeTypeString}},
			KeySchema:            []KeySchemaElement{{AttributeName: "id", KeyType: KeyTypeHash}},
		}
	}
	return tables
}
