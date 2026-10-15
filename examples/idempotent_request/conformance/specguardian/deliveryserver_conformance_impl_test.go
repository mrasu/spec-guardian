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

func (e *DeliveryServerEnvironment) BuildDeliverManualObservation(t *testing.T, fetched DeliveryServerFetchedObservation, input FizzbeeDeliveryServerState, output DeliveryServerDeliverOutput, actionErr error) DeliveryServerManualObservation {
	t.Helper()
	normalizeGeneratedDeliveryID(input, fetched.Fields.MySQL.OrderRecords, fetched.Fields.DynamoDB.DeliveryRecords)

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

func normalizeGeneratedDeliveryID(input FizzbeeDeliveryServerState, orders map[string]MySQLOrderRecordsField, deliveryRecords map[string]DynamoDBDeliveryRecordsField) {
	count := input.Fields.MySQL.DeliveryPreparedCount
	knownIDs := make(map[string]struct{})
	for _, record := range input.Fields.MySQL.OrderRecords {
		if record.DeliveryId != nil {
			knownIDs[*record.DeliveryId] = struct{}{}
		}
	}
	for key, record := range orders {
		deliveryID := record.DeliveryId
		if deliveryID == nil || *deliveryID == "" {
			continue
		}
		id := *deliveryID
		if _, known := knownIDs[id]; known {
			continue
		}
		abstractID := fmt.Sprintf("delivery-id-%d", count+1)
		record.DeliveryId = &abstractID
		orders[key] = record
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
		return "PROCESSING"
	}
}
