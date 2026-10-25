// Package errorascrashed treats every returned Action error as a model crash.
package errorascrashed

import (
	"context"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

// Config is reserved for shared result-observation settings.
type Config struct{}

// Provider reports a crash-equivalent result for every non-nil Action error.
type Provider struct {
	fields map[string]bool
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.ResultObserver = (*Provider)(nil)

// New constructs an error-as-crashed provider using the provider constructor contract.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the shared settings and field requests.
func NewProvider(_ Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	settings, err := parseConfig(config, fields)
	if err != nil {
		return nil, err
	}
	configured := make(map[string]bool, len(settings.Fields))
	for _, field := range settings.Fields {
		if field.Field == "" || configured[field.Field] {
			return nil, fmt.Errorf("invalid or duplicate error-as-crashed field %q", field.Field)
		}
		if field.Output.Type.Kind != provider.KindBool || field.Output.Type.Nullable {
			return nil, fmt.Errorf("error-as-crashed field %s requires a non-nullable bool", field.Field)
		}
		configured[field.Field] = true
	}
	return &Provider{fields: configured}, nil
}

func (o *Provider) PrepareCase(context.Context) error    { return nil }
func (o *Provider) PrepareAttempt(context.Context) error { return nil }
func (o *Provider) CleanupAttempt(context.Context) error { return nil }
func (o *Provider) CleanupCase(context.Context) error    { return nil }

// ObserveResult treats any non-nil Action error as a crash, without inspecting its type or message.
func (o *Provider) ObserveResult(_ context.Context, field string, _ any, actionErr error) (any, error) {
	if !o.fields[field] {
		return nil, fmt.Errorf("unknown error-as-crashed field %q", field)
	}
	return actionErr != nil, nil
}
