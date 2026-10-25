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

type OrderServerCreateOrderInput struct{ RequestID, Payload string }

type OrderServerCreateOrderOutput = *idempotentrequest.Order

func (e *OrderServerEnvironment) BuildCreateOrderInput(t *testing.T, input FizzbeeOrderServerState) OrderServerCreateOrderInput {
	t.Helper()
	return OrderServerCreateOrderInput{RequestID: input.Params.OrderServer.RequestId, Payload: input.Fields.OrderServer.RequestPayload}
}

func (e *OrderServerEnvironment) BuildCreateOrderManualObservation(t *testing.T, fetched OrderServerFetchedObservation, input FizzbeeOrderServerState, output OrderServerCreateOrderOutput, actionErr error) OrderServerManualObservation {
	t.Helper()
	result := "ORDERED"
	if actionErr != nil {
		result = "PROCESSING"
		if errors.Is(actionErr, idempotentrequest.ErrConflict) {
			result = "CONFLICT"
		}
	}

	return OrderServerManualObservation{
		Fields: OrderServerManualField{
			OrderServer: OrderServerOrderServerManualField{
				RequestPayload: gcmp.Observed(input.Fields.OrderServer.RequestPayload),
				Result:         gcmp.Observed(result),
			},
		},
	}
}
