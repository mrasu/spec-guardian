//go:build specguardian

package specguardian_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	idempotentrequest "spec-guardian/examples/idempotent-request"

	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gcmp"
)

var _ DeliveryServerActions = (*deliveryServerActions)(nil)

type deliveryServerActions struct {
	server *idempotentrequest.DeliveryServer
}

func newDeliveryServerActions(server *idempotentrequest.DeliveryServer) *deliveryServerActions {
	return &deliveryServerActions{server: server}
}

func (a *deliveryServerActions) Deliver(ctx context.Context, input DeliveryServerDeliverInput) (DeliveryServerDeliverOutput, error) {
	return a.server.Deliver(ctx, input.OrderID, input.Data)
}

type DeliveryServerEnvironment struct{ resources *testResources }

func NewDeliveryServerEnvironment(resources *testResources) *DeliveryServerEnvironment {
	return &DeliveryServerEnvironment{resources: resources}
}

func BuildDeliveryServerConformanceComponents(t *testing.T, controller *guardian.InjectionController) (DeliveryServerActions, *DeliveryServerEnvironment) {
	t.Helper()
	resources := newTestResources(t, controller)
	server := idempotentrequest.NewDeliveryServer(resources.actionDB, resources.actionDynamo)
	return newDeliveryServerActions(server), NewDeliveryServerEnvironment(resources)
}

func (e *DeliveryServerEnvironment) ObserveCurrentState(t *testing.T) DeliveryServerObservedState {
	t.Helper()
	return DeliveryServerObservedState{Fields: DeliveryServerObservedFields{
		DynamoDB: DeliveryServerDynamoDBObservedFields{
			DeliveryRecords:         gcmp.Observed(e.resources.observeDynamo(t, deliveryTable)),
			OrderIdempotencyRecords: gcmp.Observed(e.resources.observeDynamo(t, orderTable)),
		},
		MySQL: DeliveryServerMySQLObservedFields{
			OrderRecords: gcmp.Observed(e.resources.observeOrders(t)),
		},
	}}
}

type DeliveryServerDeliverInput struct {
	OrderID uint64
	Data    string
}

type DeliveryServerDeliverOutput = *idempotentrequest.Delivery

func (e *DeliveryServerEnvironment) SetupDeliverCase(t *testing.T) { t.Helper() }

func (e *DeliveryServerEnvironment) CleanupDeliverCase(t *testing.T) { t.Helper() }

func (e *DeliveryServerEnvironment) SetupDeliverAttempt(t *testing.T, input FizzbeeDeliveryServerState) {
	t.Helper()
	e.resources.setup(t, input.Fields.DynamoDB.OrderIdempotencyRecords, input.Fields.DynamoDB.DeliveryRecords, input.Fields.MySQL.OrderRecords, input.Fields.MySQL.NextOrderId)
}

func (e *DeliveryServerEnvironment) CleanupDeliverAttempt(t *testing.T) {
	t.Helper()
	e.resources.cleanup(t)
}

func (e *DeliveryServerEnvironment) BuildDeliverInput(t *testing.T, input FizzbeeDeliveryServerState) DeliveryServerDeliverInput {
	t.Helper()
	var orderID uint64
	if input.Fields.DeliveryServer.OrderId != nil {
		orderID = uint64(*input.Fields.DeliveryServer.OrderId)
	}
	return DeliveryServerDeliverInput{OrderID: orderID, Data: input.Fields.DeliveryServer.RequestDeliveryData}
}

func (e *DeliveryServerEnvironment) BuildObservedDeliverState(t *testing.T, observedState DeliveryServerObservedState, input FizzbeeDeliveryServerState, output DeliveryServerDeliverOutput, actionErr error) DeliveryServerObservedState {
	t.Helper()
	orders := e.resources.observeOrders(t)
	deliveryRecords := e.resources.observeDynamo(t, deliveryTable)
	normalizeGeneratedDeliveryID(input, orders, deliveryRecords)

	return DeliveryServerObservedState{
		Fields: DeliveryServerObservedFields{
			DynamoDB: DeliveryServerDynamoDBObservedFields{
				DeliveryRecords:         gcmp.Observed(deliveryRecords),
				OrderIdempotencyRecords: observedState.Fields.DynamoDB.OrderIdempotencyRecords,
			},
			MySQL: DeliveryServerMySQLObservedFields{
				DeliveryPreparedCount: gcmp.Unobservable[int]("internal property"),
				NextOrderId:           gcmp.Unobservable[int]("internal property"),
				OrderRecords:          gcmp.Observed(orders),
			},
			RequestPreparator: DeliveryServerRequestPreparatorObservedFields{
				SecondDeliveryData:            gcmp.Unobservable[string]("internal property"),
				SecondDeliveryRequestPrepared: gcmp.Unobservable[bool]("internal property"),
				SecondOrderPayload:            gcmp.Unobservable[string]("internal property"),
				SecondOrderRequestPrepared:    gcmp.Unobservable[bool]("internal property"),
			},
			DeliveryServer: DeliveryServerDeliveryServerObservedFields{
				OrderId:             gcmp.Observed(input.Fields.DeliveryServer.OrderId),
				RequestDeliveryData: gcmp.Observed(input.Fields.DeliveryServer.RequestDeliveryData),
				Result:              gcmp.Observed(deliveryResult(actionErr)),
			},
		},
	}
}

func (e *DeliveryServerEnvironment) BuildAllowedDeliverState(t *testing.T, allowed FizzbeeDeliveryServerState) DeliveryServerComparisonState {
	t.Helper()
	return DeliveryServerComparisonState{
		Fields: DeliveryServerComparisonFields{
			DynamoDB: DeliveryServerDynamoDBComparisonFields{
				DeliveryRecords:         gcmp.Equal(allowed.Fields.DynamoDB.DeliveryRecords),
				OrderIdempotencyRecords: gcmp.Equal(allowed.Fields.DynamoDB.OrderIdempotencyRecords),
			},
			MySQL: DeliveryServerMySQLComparisonFields{
				DeliveryPreparedCount: gcmp.Ignore[int]("internal property"),
				NextOrderId:           gcmp.Ignore[int]("internal property"),
				OrderRecords:          gcmp.Equal(allowed.Fields.MySQL.OrderRecords),
			},
			RequestPreparator: DeliveryServerRequestPreparatorComparisonFields{
				SecondDeliveryData:            gcmp.Ignore[string]("internal property"),
				SecondDeliveryRequestPrepared: gcmp.Ignore[bool]("internal property"),
				SecondOrderPayload:            gcmp.Ignore[string]("internal property"),
				SecondOrderRequestPrepared:    gcmp.Ignore[bool]("internal property"),
			},
			DeliveryServer: DeliveryServerDeliveryServerComparisonFields{
				OrderId:             gcmp.Equal(allowed.Fields.DeliveryServer.OrderId),
				RequestDeliveryData: gcmp.Equal(allowed.Fields.DeliveryServer.RequestDeliveryData),
				Result:              gcmp.Equal(allowed.Fields.DeliveryServer.Result),
			},
		},
	}
}

func normalizeGeneratedDeliveryID(input FizzbeeDeliveryServerState, orders, deliveryRecords map[string]any) {
	count := input.Fields.MySQL.DeliveryPreparedCount
	knownIDs := make(map[string]struct{})
	for _, raw := range input.Fields.MySQL.OrderRecords {
		if id, ok := raw.(map[string]any)["delivery_id"].(string); ok {
			knownIDs[id] = struct{}{}
		}
	}
	for _, raw := range orders {
		record := raw.(map[string]any)
		id, ok := record["delivery_id"].(string)
		if !ok || id == "" {
			continue
		}
		if _, known := knownIDs[id]; known {
			continue
		}
		abstractID := fmt.Sprintf("delivery-id-%d", count+1)
		record["delivery_id"] = abstractID
		if delivery, exists := deliveryRecords[id]; exists {
			delete(deliveryRecords, id)
			deliveryRecords[abstractID] = delivery
		}
		return
	}
}

func deliveryResult(err error) string {
	if err == nil {
		return "DELIVERED"
	}
	switch {
	case errors.Is(err, idempotentrequest.ErrConflict):
		return "CONFLICT"
	case errors.Is(err, idempotentrequest.ErrOrderNotFound):
		return "NOT_FOUND"
	case errors.Is(err, idempotentrequest.ErrAlreadyDelivered):
		return "ALREADY_DELIVERED"
	default:
		return "FAILED"
	}
}
