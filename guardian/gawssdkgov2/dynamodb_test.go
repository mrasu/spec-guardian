package gawssdkgov2

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const flociEndpoint = "http://localhost:14566"

func TestDynamoDBIntegration(t *testing.T) {
	controller := guardian.NewInjectionController("PutItem")
	client := newDynamoDBClient(controller)
	tableName := fmt.Sprintf("spec-guardian-%d", time.Now().UnixNano())

	_, err := client.CreateTable(t.Context(), &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
		require.NoError(t, err)
	})

	endAttempt := controller.InjectFaultAt(1)
	output, err := client.PutItem(t.Context(), putItemInput(tableName))
	endAttempt()
	assert.Nil(t, output)
	assert.ErrorIs(t, err, guardian.ErrInjected)
	assert.Equal(t, []guardian.IOEvent{{Number: 1, Operation: "aws.PutItem", Injected: true}}, controller.Events())
	assertItemMissing(t, client, tableName)

	endAttempt = controller.InjectFaultAt(2)
	_, err = client.PutItem(t.Context(), putItemInput(tableName))
	endAttempt()
	require.NoError(t, err)
	assert.Equal(t, []guardian.IOEvent{{Number: 1, Operation: "aws.PutItem", Injected: false}}, controller.Events())
	assertItemExists(t, client, tableName)
}

func newDynamoDBClient(fault guardian.Fault) *dynamodb.Client {
	interceptors := smithyhttp.InterceptorRegistry{}
	interceptors.AddBeforeExecution(NewInterceptor(fault))
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("test", "test", "")),
		BaseEndpoint: aws.String(flociEndpoint),
		Interceptors: interceptors,
	})
}

func putItemInput(tableName string) *dynamodb.PutItemInput {
	return &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]types.AttributeValue{
			"id":    &types.AttributeValueMemberS{Value: "1"},
			"value": &types.AttributeValueMemberS{Value: "stored"},
		},
	}
}

func assertItemMissing(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()
	output, err := client.GetItem(t.Context(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}},
	})
	require.NoError(t, err)
	assert.Empty(t, output.Item)
}

func assertItemExists(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()
	output, err := client.GetItem(t.Context(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "stored", output.Item["value"].(*types.AttributeValueMemberS).Value)
}
