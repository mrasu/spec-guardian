//go:build specguardian

// Shared infrastructure for the generated conformance tests.
package specguardian_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-sql-driver/mysql"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gawssdkgov2"
	"github.com/mrasu/spec-guardian/guardian/gsql"
	"github.com/stretchr/testify/require"
)

const (
	databaseDSN    = "root@tcp(localhost:13306)/idempotent_request?parseTime=true"
	dynamoEndpoint = "http://localhost:14566"
	dynamoRegion   = "us-east-1"
	orderTable     = "order_idempotency_records"
	deliveryTable  = "delivery_records"
	cleanupTimeout = 10 * time.Second
)

type testResources struct {
	actionDB     *sql.DB
	actionDynamo *dynamodb.Client
}

func newTestResources(t *testing.T, controller *guardian.InjectionController) *testResources {
	t.Helper()

	actionDB := openDatabase(t, controller)
	t.Cleanup(func() {
		require.NoError(t, actionDB.Close())
	})
	actionDynamo := newDynamoClient(controller)
	resources := &testResources{actionDB: actionDB, actionDynamo: actionDynamo}
	resources.cleanup(t)
	return resources
}

func openDatabase(t *testing.T, fault guardian.Fault) *sql.DB {
	t.Helper()
	configuration, err := mysql.ParseDSN(databaseDSN)
	require.NoError(t, err)
	connector, err := mysql.NewConnector(configuration)
	require.NoError(t, err)
	connector = gsql.NewConnector(connector, fault)
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)
	require.NoError(t, database.PingContext(t.Context()))
	return database
}

func newDynamoClient(fault guardian.Fault) *dynamodb.Client {
	options := dynamodb.Options{
		BaseEndpoint: aws.String(dynamoEndpoint), Region: dynamoRegion,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("test", "test", "")), Retryer: aws.NopRetryer{},
	}
	interceptors := smithyhttp.InterceptorRegistry{}
	interceptors.AddBeforeExecution(gawssdkgov2.NewInterceptor(fault))
	options.Interceptors = interceptors
	return dynamodb.New(options)
}

func (r *testResources) setup(t *testing.T, dynamoOrderRecords map[string]DynamoDBOrderIdempotencyRecordsField, dynamoDeliveryRecords map[string]DynamoDBDeliveryRecordsField, orderRecords map[string]MySQLOrderRecordsField, nextOrderID int) {
	t.Helper()
	for id, record := range orderRecords {
		_, err := r.actionDB.ExecContext(t.Context(), `INSERT INTO orders (id, request_id, payload, delivery_id, delivery_data, status) VALUES (?, ?, ?, ?, ?, ?)`, id, record.RequestId, record.Payload, record.DeliveryId, record.DeliveryData, record.Status)
		require.NoError(t, err)
	}
	_, err := r.actionDB.ExecContext(t.Context(), fmt.Sprintf("ALTER TABLE orders AUTO_INCREMENT = %d", nextOrderID+1))
	require.NoError(t, err)
	r.putItems(t, orderTable, toDynamoRecords(dynamoOrderRecords))
	r.putItems(t, deliveryTable, toDynamoDeliveryRecords(dynamoDeliveryRecords))
}

func toDynamoRecords(records map[string]DynamoDBOrderIdempotencyRecordsField) map[string]any {
	converted := make(map[string]any, len(records))
	for id, record := range records {
		var orderID any
		if record.OrderId != nil {
			orderID = *record.OrderId
		}
		converted[id] = map[string]any{"order_id": orderID, "payload": record.Payload, "status": record.Status}
	}
	return converted
}

func toDynamoDeliveryRecords(records map[string]DynamoDBDeliveryRecordsField) map[string]any {
	converted := make(map[string]any, len(records))
	for id, record := range records {
		converted[id] = map[string]any{"order_id": record.OrderId, "data": record.Data}
	}
	return converted
}

func (r *testResources) putItems(t *testing.T, table string, records map[string]any) {
	t.Helper()
	for id, raw := range records {
		item := map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: id}}
		for name, value := range raw.(map[string]any) {
			if value != nil {
				item[name] = &types.AttributeValueMemberS{Value: fmt.Sprint(value)}
			}
		}
		_, err := r.actionDynamo.PutItem(t.Context(), &dynamodb.PutItemInput{TableName: aws.String(table), Item: item})
		require.NoError(t, err)
	}
}

func (r *testResources) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	_, err := r.actionDB.ExecContext(ctx, "TRUNCATE TABLE orders")
	require.NoError(t, err)
	for _, table := range []string{orderTable, deliveryTable} {
		output, err := r.actionDynamo.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(table), ProjectionExpression: aws.String("id")})
		require.NoError(t, err)
		for _, item := range output.Items {
			_, err := r.actionDynamo.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(table), Key: map[string]types.AttributeValue{"id": item["id"]}})
			require.NoError(t, err)
		}
	}
}
