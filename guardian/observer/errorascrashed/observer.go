// Package errorascrashed treats every returned Action error as a model crash.
package errorascrashed

import (
	"context"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/observer"
)

// Config is reserved for shared result-observation settings.
type Config struct{}

// Observer reports a crash-equivalent result for every non-nil Action error.
type Observer struct {
	fields map[string]bool
}

var _ observer.ResultObserver = (*Observer)(nil)

// New constructs an error-as-crashed observer using the plugin constructor contract.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the shared settings and field requests.
func NewObserver(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	if len(config) != 0 {
		return nil, fmt.Errorf("error-as-crashed observer does not accept shared settings")
	}
	configured := make(map[string]bool, len(fields))
	for _, field := range fields {
		if field.Field == "" || configured[field.Field] {
			return nil, fmt.Errorf("invalid or duplicate error-as-crashed field %q", field.Field)
		}
		if field.Output.Type.Kind != observer.KindBool || field.Output.Type.Nullable {
			return nil, fmt.Errorf("error-as-crashed field %s requires a non-nullable bool", field.Field)
		}
		if len(field.Config) != 0 {
			return nil, fmt.Errorf("error-as-crashed field %s does not accept settings", field.Field)
		}
		configured[field.Field] = true
	}
	return &Observer{fields: configured}, nil
}

func (o *Observer) PrepareCase(context.Context) error    { return nil }
func (o *Observer) PrepareAttempt(context.Context) error { return nil }
func (o *Observer) CleanupAttempt(context.Context) error { return nil }
func (o *Observer) CleanupCase(context.Context) error    { return nil }

// Observe treats any non-nil Action error as a crash, without inspecting its type or message.
func (o *Observer) Observe(_ context.Context, field string, _ any, actionErr error) (any, error) {
	if !o.fields[field] {
		return nil, fmt.Errorf("unknown error-as-crashed field %q", field)
	}
	return actionErr != nil, nil
}
