package field

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveTables_RejectsMismatchedAttribute(t *testing.T) {
	got, err := ResolveTables([]Table{{
		Name:                 "events",
		AttributeDefinitions: []AttributeDefinition{{AttributeName: "other", AttributeType: AttributeTypeString}},
		KeySchema:            []KeySchemaElement{{AttributeName: "id", KeyType: KeyTypeHash}},
	}})
	require.ErrorContains(t, err, "matching string key")
	require.Nil(t, got)
}

func TestResolveTables_RejectsDuplicateTables(t *testing.T) {
	table := Table{
		Name:                 "events",
		AttributeDefinitions: []AttributeDefinition{{AttributeName: "id", AttributeType: AttributeTypeString}},
		KeySchema:            []KeySchemaElement{{AttributeName: "id", KeyType: KeyTypeHash}},
	}
	got, err := ResolveTables([]Table{table, table})
	require.ErrorContains(t, err, "duplicate DynamoDB table")
	require.Nil(t, got)
}
