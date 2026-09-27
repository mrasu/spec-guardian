package idempotentrequest

import "errors"

var (
	// ErrConflict reports reuse of an identity with different request data.
	ErrConflict = errors.New("request conflicts with persisted data")
	// ErrAlreadyDelivered reports a delivery request for an already delivered order.
	ErrAlreadyDelivered = errors.New("order is already delivered")
	// ErrOrderNotFound reports an unknown order ID.
	ErrOrderNotFound = errors.New("order not found")
)
