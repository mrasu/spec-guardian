package idempotentrequest

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPServerCreateOrder(t *testing.T) {
	handler, database, dynamo := newHTTPTestHandler(t)
	response := postJSON(t, handler, "/orders", `{"request_id":"request-1","payload":"order-data"}`)

	assert.Equal(t, http.StatusOK, response.Code)
	var order Order
	require.NoError(t, json.NewDecoder(response.Body).Decode(&order))
	assert.NotZero(t, order.ID)
	assert.Equal(t, OrderStatusCreated, order.Status)

	var requestID, payload string
	var status OrderStatus
	err := database.QueryRowContext(t.Context(), "SELECT request_id, payload, status FROM orders WHERE id = ?", order.ID).Scan(&requestID, &payload, &status)
	require.NoError(t, err)
	assert.Equal(t, "request-1", requestID)
	assert.Equal(t, "order-data", payload)
	assert.Equal(t, OrderStatusCreated, status)

	item, err := dynamo.GetItem(t.Context(), &dynamodb.GetItemInput{
		TableName:      aws.String(orderIdempotencyTable),
		Key:            map[string]types.AttributeValue{"id": stringAttribute("request-1")},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	payload, err = attributeString(item.Item, "payload")
	require.NoError(t, err)
	idempotencyStatus, err := attributeString(item.Item, "status")
	require.NoError(t, err)
	assert.Equal(t, "order-data", payload)
	assert.Equal(t, string(IdempotencyStatusDone), idempotencyStatus)
	assert.Equal(t, order.ID, attributeUint64(t, item.Item, "order_id"))
}

func TestHTTPServerDeliverOrder(t *testing.T) {
	handler, database, dynamo := newHTTPTestHandler(t)
	createResponse := postJSON(t, handler, "/orders", `{"request_id":"request-1","payload":"order-data"}`)
	require.Equal(t, http.StatusOK, createResponse.Code)
	var order Order
	require.NoError(t, json.NewDecoder(createResponse.Body).Decode(&order))

	deliverResponse := postJSON(t, handler, "/orders/"+strconv.FormatUint(order.ID, 10)+"/deliver", `{"data":"delivery-data"}`)
	assert.Equal(t, http.StatusOK, deliverResponse.Code)
	var delivery Delivery
	require.NoError(t, json.NewDecoder(deliverResponse.Body).Decode(&delivery))
	require.NotEmpty(t, delivery.ID)
	assert.Equal(t, OrderStatusDelivered, delivery.Status)

	var deliveryID, deliveryData string
	var status OrderStatus
	err := database.QueryRowContext(t.Context(), "SELECT delivery_id, delivery_data, status FROM orders WHERE id = ?", order.ID).Scan(&deliveryID, &deliveryData, &status)
	require.NoError(t, err)
	assert.Equal(t, delivery.ID, deliveryID)
	assert.Equal(t, "delivery-data", deliveryData)
	assert.Equal(t, OrderStatusDelivered, status)

	item, err := dynamo.GetItem(t.Context(), &dynamodb.GetItemInput{
		TableName:      aws.String(deliveryTable),
		Key:            map[string]types.AttributeValue{"id": stringAttribute(delivery.ID)},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	data, err := attributeString(item.Item, "data")
	require.NoError(t, err)
	assert.Equal(t, order.ID, attributeUint64(t, item.Item, "order_id"))
	assert.Equal(t, "delivery-data", data)
}

func newHTTPTestHandler(t *testing.T) (http.Handler, *sql.DB, *dynamodb.Client) {
	t.Helper()
	configuration, err := mysql.ParseDSN(databaseDSN)
	require.NoError(t, err)
	connector, err := mysql.NewConnector(configuration)
	require.NoError(t, err)
	database := sql.OpenDB(connector)
	require.NoError(t, database.PingContext(t.Context()))
	dynamo := NewDynamoDBClient()
	cleanupHTTPTestData(t, database, dynamo)
	t.Cleanup(func() {
		cleanupHTTPTestData(t, database, dynamo)
		require.NoError(t, database.Close())
	})
	server := NewHTTPServer(NewOrderServer(database, dynamo), NewDeliveryServer(database, dynamo))
	return server.Handler(), database, dynamo
}

func cleanupHTTPTestData(t *testing.T, database *sql.DB, dynamo *dynamodb.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := database.ExecContext(ctx, "TRUNCATE TABLE orders")
	require.NoError(t, err)
	for _, table := range []string{orderIdempotencyTable, deliveryTable} {
		output, err := dynamo.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(table), ProjectionExpression: aws.String("id")})
		require.NoError(t, err)
		for _, item := range output.Items {
			_, err := dynamo.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(table), Key: map[string]types.AttributeValue{"id": item["id"]}})
			require.NoError(t, err)
		}
	}
}

func postJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func attributeUint64(t *testing.T, item map[string]types.AttributeValue, name string) uint64 {
	t.Helper()
	value, err := attributeString(item, name)
	require.NoError(t, err)
	number, err := strconv.ParseUint(value, 10, 64)
	require.NoError(t, err)
	return number
}
