// Package actionerror observes unexpected errors separately from business results.
package actionerror

import (
	"context"
	"errors"

	idempotentrequest "spec-guardian/examples/idempotent-request"

	"github.com/mrasu/spec-guardian/guardian/observer"
	builtin "github.com/mrasu/spec-guardian/guardian/observer/errorascrashed"
)

// Config is reserved for result-observation settings.
type Config struct{}

// Observer excludes business errors represented by the model's result fields.
type Observer struct {
	base *builtin.Observer
}

var _ observer.ResultObserver = (*Observer)(nil)

// New constructs the observer using the plugin constructor contract.
func New(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(config, fields)
}

// NewObserver validates settings and field requests with the builtin observer.
func NewObserver(config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	base, err := builtin.New(builtin.Config{}, config, fields)
	if err != nil {
		return nil, err
	}
	return &Observer{base: base}, nil
}

func (o *Observer) PrepareCase(ctx context.Context) error    { return o.base.PrepareCase(ctx) }
func (o *Observer) PrepareAttempt(ctx context.Context) error { return o.base.PrepareAttempt(ctx) }
func (o *Observer) CleanupAttempt(ctx context.Context) error { return o.base.CleanupAttempt(ctx) }
func (o *Observer) CleanupCase(ctx context.Context) error    { return o.base.CleanupCase(ctx) }

// Observe leaves business errors to the separately observed result fields.
func (o *Observer) Observe(ctx context.Context, field string, output any, actionErr error) (any, error) {
	if errors.Is(actionErr, idempotentrequest.ErrConflict) ||
		errors.Is(actionErr, idempotentrequest.ErrOrderNotFound) ||
		errors.Is(actionErr, idempotentrequest.ErrAlreadyDelivered) {
		actionErr = nil
	}
	return o.base.Observe(ctx, field, output, actionErr)
}
