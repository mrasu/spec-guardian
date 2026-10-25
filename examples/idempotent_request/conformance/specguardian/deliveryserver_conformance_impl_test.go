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

type DeliveryServerDeliverInput struct {
	OrderID uint64
	Data    string
}

type DeliveryServerDeliverOutput = *idempotentrequest.Delivery

func (e *DeliveryServerEnvironment) BuildDeliverInput(t *testing.T, input FizzbeeDeliveryServerState) DeliveryServerDeliverInput {
	t.Helper()
	var orderID uint64
	if input.Fields.DeliveryServer.OrderId != nil {
		orderID = uint64(*input.Fields.DeliveryServer.OrderId)
	}
	return DeliveryServerDeliverInput{OrderID: orderID, Data: input.Fields.DeliveryServer.RequestDeliveryData}
}

func (e *DeliveryServerEnvironment) BuildDeliverManualObservation(t *testing.T, fetched DeliveryServerFetchedObservation, input FizzbeeDeliveryServerState, output DeliveryServerDeliverOutput, actionErr error) DeliveryServerManualObservation {
	t.Helper()

	return DeliveryServerManualObservation{
		Fields: DeliveryServerManualField{
			DeliveryServer: DeliveryServerDeliveryServerManualField{
				OrderId:             gcmp.Observed(input.Fields.DeliveryServer.OrderId),
				RequestDeliveryData: gcmp.Observed(input.Fields.DeliveryServer.RequestDeliveryData),
				Result:              gcmp.Observed(deliveryResult(actionErr)),
			},
		},
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
		return "PROCESSING"
	}
}
