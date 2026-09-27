package idempotentrequest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// OrderStatus represents an order's delivery state.
type OrderStatus string

const (
	OrderStatusCreated    OrderStatus = "CREATED"
	OrderStatusDelivering OrderStatus = "DELIVERING"
	OrderStatusDelivered  OrderStatus = "DELIVERED"
)

// IdempotencyStatus represents the state of an idempotent request.
type IdempotencyStatus string

const (
	IdempotencyStatusProcessing IdempotencyStatus = "PROCESSING"
	IdempotencyStatusDone       IdempotencyStatus = "DONE"
)

// Order is the result of CreateOrder.
type Order struct {
	ID        uint64      `json:"id"`
	RequestID string      `json:"request_id"`
	Payload   string      `json:"payload"`
	Status    OrderStatus `json:"status"`
}

// OrderServer implements the order idempotency workflow.
type OrderServer struct {
	database *sql.DB
	dynamo   dynamoDBAPI
}

// NewOrderServer returns an OrderServer with validated dependencies.
func NewOrderServer(database *sql.DB, dynamo dynamoDBAPI) *OrderServer {
	if database == nil || dynamo == nil {
		panic("idempotentrequest: OrderServer dependencies must not be nil")
	}

	return &OrderServer{database: database, dynamo: dynamo}
}

// CreateOrder creates an order or recovers an interrupted request.
func (s *OrderServer) CreateOrder(ctx context.Context, requestID, payload string) (*Order, error) {
	if requestID == "" || payload == "" {
		return nil, errors.New("request ID and payload must not be empty")
	}
	if err := s.beginOrderIdempotentRequest(ctx, requestID, payload); err != nil {
		return nil, err
	}

	order, err := s.createOrFindOrder(ctx, requestID, payload)
	if err != nil {
		return nil, err
	}

	if err := s.endOrderIdempotentRequest(ctx, requestID, order.ID); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderServer) beginOrderIdempotentRequest(ctx context.Context, requestID, payload string) error {
	_, err := s.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(orderIdempotencyTable),
		Item: map[string]types.AttributeValue{
			"id":      stringAttribute(requestID),
			"payload": stringAttribute(payload),
			"status":  stringAttribute(string(IdempotencyStatusProcessing)),
		},
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err == nil {
		return nil
	}
	if !isConditionalFailure(err) {
		return fmt.Errorf("create PROCESSING order idempotency record: %w", err)
	}

	item, err := s.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(orderIdempotencyTable),
		Key:            map[string]types.AttributeValue{"id": stringAttribute(requestID)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("read order idempotency record: %w", err)
	}
	storedPayload, err := attributeString(item.Item, "payload")
	if err != nil {
		return err
	}
	if storedPayload != payload {
		return ErrConflict
	}
	return nil
}

func (s *OrderServer) createOrFindOrder(ctx context.Context, requestID, payload string) (*Order, error) {
	order, err := s.findOrder(ctx, requestID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		return s.insertOrder(ctx, requestID, payload)
	}

	if order.Payload != payload {
		return nil, ErrConflict
	}
	return order, nil
}

func (s *OrderServer) endOrderIdempotentRequest(ctx context.Context, requestID string, orderID uint64) error {
	_, err := s.dynamo.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(orderIdempotencyTable), Key: map[string]types.AttributeValue{"id": stringAttribute(requestID)},
		UpdateExpression:         aws.String("SET #status = :done, order_id = :order_id"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":done":     stringAttribute(string(IdempotencyStatusDone)),
			":order_id": stringAttribute(fmt.Sprint(orderID)),
		},
	})
	if err != nil {
		return fmt.Errorf("mark order idempotency record DONE: %w", err)
	}
	return nil
}

func (s *OrderServer) findOrder(ctx context.Context, requestID string) (*Order, error) {
	order := &Order{}
	err := s.database.QueryRowContext(ctx, "SELECT id, request_id, payload, status FROM orders WHERE request_id = ?", requestID).Scan(&order.ID, &order.RequestID, &order.Payload, &order.Status)
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderServer) insertOrder(ctx context.Context, requestID, payload string) (*Order, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin order transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, "INSERT INTO orders (request_id, payload, status) VALUES (?, ?, ?)", requestID, payload, OrderStatusCreated)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read order ID: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
	}

	return &Order{ID: uint64(id), RequestID: requestID, Payload: payload, Status: OrderStatusCreated}, nil
}
