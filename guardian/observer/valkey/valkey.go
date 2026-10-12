// Package valkey observes string records stored in Valkey.
package valkey

import (
	"context"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/observer"
	valkeygo "github.com/valkey-io/valkey-go"
)

const mgetBatchSize = 128

// Config supplies an existing client when one is available.
type Config struct {
	Client valkeygo.Client
}

// Observer reads all keys selected for each field.
type Observer struct {
	client valkeygo.Client
	owned  bool
	fields map[string]string
}

var _ observer.Observer = (*Observer)(nil)

// New creates the built-in Valkey observer.
func New(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	return NewObserver(dynamic, config, fields)
}

// NewObserver validates the requests and creates a client if needed.
func NewObserver(dynamic Config, config map[string]any, fields []observer.FieldRequest) (*Observer, error) {
	patterns, err := validateFields(fields)
	if err != nil {
		return nil, err
	}

	address, ok := config["address"].(string)
	if !ok || address == "" {
		return nil, fmt.Errorf("Valkey address must be a nonempty string")
	}

	client := dynamic.Client
	owned := false
	if client == nil {
		client, err = valkeygo.NewClient(valkeygo.ClientOption{InitAddress: []string{address}})
		if err != nil {
			return nil, fmt.Errorf("open Valkey client: %w", err)
		}
		owned = true
	}

	return &Observer{client: client, owned: owned, fields: patterns}, nil
}

func validateFields(fields []observer.FieldRequest) (map[string]string, error) {
	patterns := make(map[string]string, len(fields))
	for _, field := range fields {
		if field.Field == "" {
			return nil, fmt.Errorf("Valkey field is empty")
		}
		if _, exists := patterns[field.Field]; exists {
			return nil, fmt.Errorf("duplicate Valkey field %q", field.Field)
		}

		if field.Output.Type.Kind != observer.KindMap || field.Output.Type.Values == nil || field.Output.Type.Values.Kind != observer.KindString {
			return nil, fmt.Errorf("Valkey field %s requires a map of strings", field.Field)
		}

		pattern, ok := field.Config["keys"].(string)
		if !ok || pattern == "" {
			return nil, fmt.Errorf("Valkey field %s requires keys", field.Field)
		}

		patterns[field.Field] = pattern
	}

	return patterns, nil
}

// PrepareCase begins a case.
func (o *Observer) PrepareCase(context.Context) error { return nil }

// PrepareAttempt begins an attempt.
func (o *Observer) PrepareAttempt(context.Context) error { return nil }

// CleanupAttempt ends an attempt.
func (o *Observer) CleanupAttempt(context.Context) error { return nil }

// CleanupCase closes a client owned by the observer.
func (o *Observer) CleanupCase(context.Context) error {
	if o.owned {
		o.client.Close()
	}
	return nil
}

// Observe scans keys and reads their string values in bounded MGET batches.
func (o *Observer) Observe(ctx context.Context, field string) (any, error) {
	pattern, ok := o.fields[field]
	if !ok {
		return nil, fmt.Errorf("unknown Valkey field %q", field)
	}

	keys, err := o.scanKeys(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("scan Valkey field %s: %w", field, err)
	}

	records, err := o.readValues(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("read Valkey field %s: %w", field, err)
	}

	return records, nil
}

func (o *Observer) scanKeys(ctx context.Context, pattern string) ([]string, error) {
	var keys []string
	seen := make(map[string]struct{})
	var cursor uint64

	for {
		entry, err := o.client.Do(ctx, o.client.B().Scan().Cursor(cursor).Match(pattern).Build()).AsScanEntry()
		if err != nil {
			return nil, err
		}

		for _, key := range entry.Elements {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}

		cursor = entry.Cursor
		if cursor == 0 {
			return keys, nil
		}
	}
}

func (o *Observer) readValues(ctx context.Context, keys []string) (map[string]string, error) {
	records := make(map[string]string, len(keys))
	for start := 0; start < len(keys); start += mgetBatchSize {
		end := min(start+mgetBatchSize, len(keys))
		batch := keys[start:end]

		values, err := valkeygo.MGet(o.client, ctx, batch)
		if err != nil {
			return nil, err
		}

		for _, key := range batch {
			value, exists := values[key]
			if !exists || value.IsNil() {
				return nil, fmt.Errorf("key %q disappeared during observation", key)
			}

			stringValue, err := value.ToString()
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}

			records[key] = stringValue
		}
	}

	return records, nil
}
