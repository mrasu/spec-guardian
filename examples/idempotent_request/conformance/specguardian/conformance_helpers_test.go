//go:build specguardian

// Shared infrastructure for the generated conformance tests.
package specguardian_test

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
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

func (r *testResources) setup(t *testing.T, dynamoOrderRecords, dynamoDeliveryRecords, orderRecords map[string]any, nextOrderID int) {
	t.Helper()
	for id, raw := range orderRecords {
		record := raw.(map[string]any)
		_, err := r.actionDB.ExecContext(t.Context(), `INSERT INTO orders (id, request_id, payload, delivery_id, delivery_data, status) VALUES (?, ?, ?, ?, ?, ?)`, id, record["request_id"], record["payload"], record["delivery_id"], record["delivery_data"], record["status"])
		require.NoError(t, err)
	}
	_, err := r.actionDB.ExecContext(t.Context(), fmt.Sprintf("ALTER TABLE orders AUTO_INCREMENT = %d", nextOrderID+1))
	require.NoError(t, err)
	r.putItems(t, orderTable, dynamoOrderRecords)
	r.putItems(t, deliveryTable, dynamoDeliveryRecords)
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

func (r *testResources) observeOrders(t *testing.T) map[string]any {
	t.Helper()
	rows, err := r.actionDB.QueryContext(t.Context(), "SELECT id, request_id, payload, delivery_id, delivery_data, status FROM orders ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	records := make(map[string]any)
	for rows.Next() {
		var id int
		var requestID, payload, status string
		var deliveryID, deliveryData sql.NullString
		require.NoError(t, rows.Scan(&id, &requestID, &payload, &deliveryID, &deliveryData, &status))
		records[strconv.Itoa(id)] = map[string]any{"request_id": requestID, "payload": payload, "delivery_id": nullableString(deliveryID), "delivery_data": nullableString(deliveryData), "status": status}
	}
	require.NoError(t, rows.Err())
	return records
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func (r *testResources) observeDynamo(t *testing.T, table string) map[string]any {
	t.Helper()
	output, err := r.actionDynamo.Scan(t.Context(), &dynamodb.ScanInput{TableName: aws.String(table)})
	require.NoError(t, err)
	records := make(map[string]any)
	for _, item := range output.Items {
		id := item["id"].(*types.AttributeValueMemberS).Value
		record := make(map[string]any)
		if table == orderTable {
			record["order_id"] = nil
		}
		for name, attribute := range item {
			if name == "id" {
				continue
			}
			value := attribute.(*types.AttributeValueMemberS).Value
			if name == "order_id" {
				number, parseErr := strconv.Atoi(value)
				require.NoError(t, parseErr)
				record[name] = float64(number)
			} else {
				record[name] = value
			}
		}
		records[id] = record
	}
	return records
}
