package idempotentrequest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// Delivery is the result of Deliver.
type Delivery struct {
	ID      string      `json:"id"`
	OrderID uint64      `json:"order_id"`
	Data    string      `json:"data"`
	Status  OrderStatus `json:"status"`
}

// DeliveryServer implements delivery and recovery.
type DeliveryServer struct {
	database *sql.DB
	dynamo   dynamoDBAPI
	newID    func() string
}

// NewDeliveryServer returns a DeliveryServer with validated dependencies.
func NewDeliveryServer(database *sql.DB, dynamo dynamoDBAPI) *DeliveryServer {
	if database == nil || dynamo == nil {
		panic("idempotentrequest: DeliveryServer dependencies must not be nil")
	}

	return &DeliveryServer{database: database, dynamo: dynamo, newID: func() string { return uuid.NewString() }}
}

// Deliver delivers an order or recovers a DELIVERING order.
func (s *DeliveryServer) Deliver(ctx context.Context, orderID uint64, data string) (*Delivery, error) {
	if orderID == 0 || data == "" {
		return nil, errors.New("order ID and delivery data must not be empty")
	}

	delivery, err := s.markOrderDelivering(ctx, orderID, data)
	if err != nil {
		return nil, err
	}
	if err := s.createDeliveryRecordInDynamo(ctx, delivery); err != nil {
		return nil, err
	}
	if err := s.markOrderDelivered(ctx, orderID); err != nil {
		return nil, err
	}

	delivery.Status = OrderStatusDelivered
	return delivery, nil
}

func (s *DeliveryServer) markOrderDelivering(ctx context.Context, orderID uint64, data string) (*Delivery, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin delivery transaction: %w", err)
	}
	defer tx.Rollback()

	var status OrderStatus
	var deliveryID, storedData sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT status, delivery_id, delivery_data FROM orders WHERE id = ? FOR UPDATE", orderID).Scan(&status, &deliveryID, &storedData)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read order for delivery: %w", err)
	}

	if status == OrderStatusDelivered {
		return nil, ErrAlreadyDelivered
	}
	if status == OrderStatusCreated {
		deliveryID = sql.NullString{String: s.newID(), Valid: true}
		storedData = sql.NullString{String: data, Valid: true}
		if _, err := tx.ExecContext(ctx, "UPDATE orders SET delivery_id = ?, delivery_data = ?, status = ? WHERE id = ?", deliveryID.String, data, OrderStatusDelivering, orderID); err != nil {
			return nil, fmt.Errorf("prepare delivery: %w", err)
		}
	} else if status != OrderStatusDelivering {
		return nil, fmt.Errorf("unknown order status %q", status)
	}

	if !storedData.Valid || storedData.String != data {
		return nil, ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit delivery transaction: %w", err)
	}
	return &Delivery{ID: deliveryID.String, OrderID: orderID, Data: data, Status: OrderStatusDelivering}, nil
}

func (s *DeliveryServer) createDeliveryRecordInDynamo(ctx context.Context, delivery *Delivery) error {
	_, err := s.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(deliveryTable),
		Item:                deliveryItem(delivery),
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err == nil {
		return nil
	}
	if !isConditionalFailure(err) {
		return fmt.Errorf("create delivery record: %w", err)
	}

	item, err := s.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(deliveryTable),
		Key:            map[string]types.AttributeValue{"id": stringAttribute(delivery.ID)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("read delivery record: %w", err)
	}

	storedData, err := attributeString(item.Item, "data")
	if err != nil {
		return err
	}
	if storedData != delivery.Data {
		return ErrConflict
	}
	return nil
}

func deliveryItem(delivery *Delivery) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"id":       stringAttribute(delivery.ID),
		"order_id": stringAttribute(fmt.Sprint(delivery.OrderID)),
		"data":     stringAttribute(delivery.Data),
	}
}

func (s *DeliveryServer) markOrderDelivered(ctx context.Context, orderID uint64) error {
	result, err := s.database.ExecContext(ctx, "UPDATE orders SET status = ? WHERE id = ? AND status = ?", OrderStatusDelivered, orderID, OrderStatusDelivering)
	if err != nil {
		return fmt.Errorf("complete delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read completed row count: %w", err)
	}
	if affected != 1 {
		return errors.New("order left DELIVERING before completion")
	}
	return nil
}
