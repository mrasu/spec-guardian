package idempotentrequest

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	orderIdempotencyTable = "order_idempotency_records"
	deliveryTable         = "delivery_records"
)

func stringAttribute(value string) *types.AttributeValueMemberS {
	return &types.AttributeValueMemberS{Value: value}
}

func attributeString(item map[string]types.AttributeValue, name string) (string, error) {
	value, ok := item[name].(*types.AttributeValueMemberS)
	if !ok {
		return "", fmt.Errorf("DynamoDB item attribute %q is missing or not a string", name)
	}
	return value.Value, nil
}

func isConditionalFailure(err error) bool {
	var failure *types.ConditionalCheckFailedException
	return errors.As(err, &failure)
}
