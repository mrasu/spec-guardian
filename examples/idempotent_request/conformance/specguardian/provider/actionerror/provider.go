// Package actionerror observes unexpected errors separately from business results.
package actionerror

import (
	"context"
	"errors"

	idempotentrequest "spec-guardian/examples/idempotent-request"

	"github.com/mrasu/spec-guardian/guardian/provider"
	builtin "github.com/mrasu/spec-guardian/guardian/provider/errorascrashed"
)

// Config is reserved for result-observation settings.
type Config struct{}

// Provider excludes business errors represented by the model's result fields.
type Provider struct {
	base *builtin.Provider
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.ResultObserver = (*Provider)(nil)

// New constructs the provider using the provider constructor contract.
func New(_ Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(config, fields)
}

// NewProvider validates settings and field requests with the builtin provider.
func NewProvider(config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	base, err := builtin.New(builtin.Config{}, config, fields)
	if err != nil {
		return nil, err
	}
	return &Provider{base: base}, nil
}

func (o *Provider) PrepareCase(ctx context.Context) error    { return o.base.PrepareCase(ctx) }
func (o *Provider) PrepareAttempt(ctx context.Context) error { return o.base.PrepareAttempt(ctx) }
func (o *Provider) CleanupAttempt(ctx context.Context) error { return o.base.CleanupAttempt(ctx) }
func (o *Provider) CleanupCase(ctx context.Context) error    { return o.base.CleanupCase(ctx) }

// ObserveResult leaves business errors to the separately observed result fields.
func (o *Provider) ObserveResult(ctx context.Context, field string, output any, actionErr error) (any, error) {
	if errors.Is(actionErr, idempotentrequest.ErrConflict) ||
		errors.Is(actionErr, idempotentrequest.ErrOrderNotFound) ||
		errors.Is(actionErr, idempotentrequest.ErrAlreadyDelivered) {
		actionErr = nil
	}
	return o.base.ObserveResult(ctx, field, output, actionErr)
}
