//go:build specguardian

package specguardian_test

import (
	"context"
	"errors"
	"testing"

	idempotentrequest "spec-guardian/examples/idempotent-request"

	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gcmp"
)

var _ OrderServerActions = (*orderServerActions)(nil)

type orderServerActions struct {
	server *idempotentrequest.OrderServer
}

func newOrderServerActions(server *idempotentrequest.OrderServer) *orderServerActions {
	return &orderServerActions{server: server}
}

func (a *orderServerActions) CreateOrder(ctx context.Context, input OrderServerCreateOrderInput) (OrderServerCreateOrderOutput, error) {
	return a.server.CreateOrder(ctx, input.RequestID, input.Payload)
}

type OrderServerEnvironment struct{ resources *testResources }

func NewOrderServerEnvironment(resources *testResources) *OrderServerEnvironment {
	return &OrderServerEnvironment{resources: resources}
}

func BuildOrderServerConformanceComponents(t *testing.T, controller *guardian.InjectionController) (OrderServerActions, *OrderServerEnvironment) {
	t.Helper()
	resources := newTestResources(t, controller)
	return newOrderServerActions(idempotentrequest.NewOrderServer(resources.actionDB, resources.actionDynamo)), NewOrderServerEnvironment(resources)
}

func (e *OrderServerEnvironment) ObserveCurrentState(t *testing.T) OrderServerObservedState {
	t.Helper()
	return OrderServerObservedState{
		Fields: OrderServerObservedFields{
			DynamoDB: OrderServerDynamoDBObservedFields{
				DeliveryRecords:         gcmp.Observed(e.resources.observeDynamo(t, deliveryTable)),
				OrderIdempotencyRecords: gcmp.Observed(e.resources.observeDynamo(t, orderTable)),
			},
			MySQL: OrderServerMySQLObservedFields{
				DeliveryPreparedCount: gcmp.Unobservable[int]("internal property"),
				NextOrderId:           gcmp.Unobservable[int]("internal property"),
				OrderRecords:          gcmp.Observed(e.resources.observeOrders(t)),
			},
		},
	}
}

type OrderServerCreateOrderInput struct{ RequestID, Payload string }

type OrderServerCreateOrderOutput = *idempotentrequest.Order

func (e *OrderServerEnvironment) SetupCreateOrderCase(t *testing.T) { t.Helper() }

func (e *OrderServerEnvironment) CleanupCreateOrderCase(t *testing.T) { t.Helper() }

func (e *OrderServerEnvironment) SetupCreateOrderAttempt(t *testing.T, input FizzbeeOrderServerState) {
	t.Helper()
	e.resources.setup(t, input.Fields.DynamoDB.OrderIdempotencyRecords, input.Fields.DynamoDB.DeliveryRecords, input.Fields.MySQL.OrderRecords, input.Fields.MySQL.NextOrderId)
}

func (e *OrderServerEnvironment) CleanupCreateOrderAttempt(t *testing.T) {
	t.Helper()
	e.resources.cleanup(t)
}

func (e *OrderServerEnvironment) BuildCreateOrderInput(t *testing.T, input FizzbeeOrderServerState) OrderServerCreateOrderInput {
	t.Helper()
	return OrderServerCreateOrderInput{RequestID: input.Params.OrderServer.RequestId, Payload: input.Fields.OrderServer.RequestPayload}
}

func (e *OrderServerEnvironment) BuildObservedCreateOrderState(t *testing.T, observedState OrderServerObservedState, input FizzbeeOrderServerState, output OrderServerCreateOrderOutput, actionErr error) OrderServerObservedState {
	t.Helper()
	orderedID := input.Fields.OrderServer.OrderedId
	if output != nil {
		orderedID = new(int(output.ID))
	}
	result := "ORDERED"
	if actionErr != nil {
		result = "FAILED"
		if errors.Is(actionErr, idempotentrequest.ErrConflict) {
			result = "CONFLICT"
		}
	}

	return OrderServerObservedState{
		Params: OrderServerObservedParams{
			OrderServer: OrderServerOrderServerObservedParams{
				RequestId: gcmp.Observed(input.Params.OrderServer.RequestId),
			},
		},
		Fields: OrderServerObservedFields{
			DynamoDB: OrderServerDynamoDBObservedFields{
				DeliveryRecords:         observedState.Fields.DynamoDB.DeliveryRecords,
				OrderIdempotencyRecords: observedState.Fields.DynamoDB.OrderIdempotencyRecords,
			},
			MySQL: OrderServerMySQLObservedFields{
				DeliveryPreparedCount: observedState.Fields.MySQL.DeliveryPreparedCount,
				NextOrderId:           observedState.Fields.MySQL.NextOrderId,
				OrderRecords:          observedState.Fields.MySQL.OrderRecords,
			},
			RequestPreparator: OrderServerRequestPreparatorObservedFields{
				SecondDeliveryData:            gcmp.Unobservable[string]("internal property"),
				SecondDeliveryRequestPrepared: gcmp.Unobservable[bool]("internal property"),
				SecondOrderPayload:            gcmp.Unobservable[string]("internal property"),
				SecondOrderRequestPrepared:    gcmp.Unobservable[bool]("internal property"),
			},
			OrderServer: OrderServerOrderServerObservedFields{
				OrderedId:      gcmp.Observed(orderedID),
				RequestPayload: gcmp.Observed(input.Fields.OrderServer.RequestPayload),
				Result:         gcmp.Observed(result),
			},
		},
	}
}

func (e *OrderServerEnvironment) BuildAllowedCreateOrderState(t *testing.T, allowed FizzbeeOrderServerState) OrderServerComparisonState {
	t.Helper()
	return OrderServerComparisonState{
		Params: OrderServerComparisonParams{
			OrderServer: OrderServerOrderServerComparisonParams{
				RequestId: gcmp.Equal(allowed.Params.OrderServer.RequestId),
			},
		},
		Fields: OrderServerComparisonFields{
			DynamoDB: OrderServerDynamoDBComparisonFields{
				DeliveryRecords:         gcmp.Equal(allowed.Fields.DynamoDB.DeliveryRecords),
				OrderIdempotencyRecords: gcmp.Equal(allowed.Fields.DynamoDB.OrderIdempotencyRecords),
			},
			MySQL: OrderServerMySQLComparisonFields{
				DeliveryPreparedCount: gcmp.Ignore[int]("internal property"),
				NextOrderId:           gcmp.Ignore[int]("internal property"),
				OrderRecords:          gcmp.Equal(allowed.Fields.MySQL.OrderRecords),
			},
			RequestPreparator: OrderServerRequestPreparatorComparisonFields{
				SecondDeliveryData:            gcmp.Ignore[string]("internal property"),
				SecondDeliveryRequestPrepared: gcmp.Ignore[bool]("internal property"),
				SecondOrderPayload:            gcmp.Ignore[string]("internal property"),
				SecondOrderRequestPrepared:    gcmp.Ignore[bool]("internal property"),
			},
			OrderServer: OrderServerOrderServerComparisonFields{
				OrderedId:      gcmp.Equal(allowed.Fields.OrderServer.OrderedId),
				RequestPayload: gcmp.Equal(allowed.Fields.OrderServer.RequestPayload),
				Result:         gcmp.Equal(allowed.Fields.OrderServer.Result),
			},
		},
	}
}
