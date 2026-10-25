// Package valkey prepares and observes string records stored in Valkey.
package valkey

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mrasu/spec-guardian/guardian/provider"
	valkeygo "github.com/valkey-io/valkey-go"
)

const mgetBatchSize = 128

// Config supplies an existing client when one is available.
type Config struct {
	Client valkeygo.Client
}

// Provider prepares and reads all keys selected for each field.
type Provider struct {
	client valkeygo.Client
	owned  bool
	fields map[string]fieldScope
}

var _ provider.Lifecycle = (*Provider)(nil)

var _ provider.Seeder = (*Provider)(nil)

var _ provider.Observer = (*Provider)(nil)

// New creates the built-in Valkey provider.
func New(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	return NewProvider(dynamic, config, fields)
}

// NewProvider validates the requests and creates a client if needed.
// Fields referencing config.keyspaces through keyspace are seeded and isolated.
// Existing keys fields remain observation-only and need no keyspace declaration.
func NewProvider(dynamic Config, config map[string]any, fields []provider.FieldRequest) (*Provider, error) {
	settings, err := parseConfig(config, fields)
	if err != nil {
		return nil, err
	}
	scopes, err := resolveFields(settings.Fields, settings.Keyspaces)
	if err != nil {
		return nil, err
	}

	client := dynamic.Client
	owned := false
	if client == nil {
		client, err = valkeygo.NewClient(valkeygo.ClientOption{InitAddress: []string{settings.Address}})
		if err != nil {
			return nil, fmt.Errorf("open Valkey client: %w", err)
		}
		owned = true
	}

	return &Provider{client: client, owned: owned, fields: scopes}, nil
}

func resolveFields(fields []fieldRequest, declarations []keyspace) (map[string]fieldScope, error) {
	keyspaces, err := resolveKeyspaces(declarations)
	if err != nil {
		return nil, err
	}
	scopes := make(map[string]fieldScope, len(fields))
	for _, field := range fields {
		if field.Field == "" {
			return nil, fmt.Errorf("Valkey field is empty")
		}
		if _, exists := scopes[field.Field]; exists {
			return nil, fmt.Errorf("duplicate Valkey field %q", field.Field)
		}
		spec := field.Output.Type
		if spec.Kind != provider.KindMap || spec.Nullable || spec.Values == nil || spec.Values.Kind != provider.KindString || spec.Values.Nullable {
			return nil, fmt.Errorf("Valkey field %s requires a non-nullable map of non-nullable strings", field.Field)
		}
		scope, err := resolveFieldScope(field, keyspaces)
		if err != nil {
			return nil, err
		}
		scopes[field.Field] = scope
	}
	return scopes, nil
}

// PrepareCase begins a case.
func (o *Provider) PrepareCase(context.Context) error { return nil }

// PrepareAttempt leaves existing data unchanged.
func (o *Provider) PrepareAttempt(context.Context) error { return nil }

// CleanupAttempt removes keys including unexpected types and partial seeds.
func (o *Provider) CleanupAttempt(ctx context.Context) error { return o.clearSeedKeyspaces(ctx) }

// CleanupCase closes a client owned by the provider.
func (o *Provider) CleanupCase(context.Context) error {
	if o.owned {
		o.client.Close()
	}
	return nil
}

// Observe scans keys and reads their string values in bounded MGET batches.
func (o *Provider) Observe(ctx context.Context, field string) (any, error) {
	scope, ok := o.fields[field]
	if !ok {
		return nil, fmt.Errorf("unknown Valkey field %q", field)
	}

	keys, err := o.scanKeys(ctx, scope.Pattern)
	if err != nil {
		return nil, fmt.Errorf("scan Valkey field %s: %w", field, err)
	}

	records, err := o.readValues(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("read Valkey field %s: %w", field, err)
	}

	if scope.Seed {
		modelRecords := make(map[string]string, len(records))
		for key, value := range records {
			modelRecords[strings.TrimPrefix(key, scope.Prefix)] = value
		}
		return modelRecords, nil
	}
	return records, nil
}

func (o *Provider) scanKeys(ctx context.Context, pattern string) ([]string, error) {
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

func (o *Provider) readValues(ctx context.Context, keys []string) (map[string]string, error) {
	records := make(map[string]string, len(keys))
	for _, key := range keys {
		kind, err := o.client.Do(ctx, o.client.B().Type().Key(key).Build()).ToString()
		if err != nil {
			return nil, err
		}
		if dataType(kind) != dataTypeString {
			return nil, fmt.Errorf("key %q has type %q; expected string", key, kind)
		}
	}
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

// Seed writes keyspace values and passes observation-only fields through unchanged.
func (o *Provider) Seed(ctx context.Context, request provider.SeedRequest) (provider.SeedResult, error) {
	records, err := o.buildSeedRecords(request)
	if err != nil {
		return provider.SeedResult{}, err
	}
	for _, value := range request.Values {
		for key, record := range records[value.Field] {
			if err := o.client.Do(ctx, o.client.B().Set().Key(key).Value(record).Build()).Error(); err != nil {
				return provider.SeedResult{}, fmt.Errorf("Valkey seed %s key %q: %w", value.Field, key, err)
			}
		}
	}
	return provider.SeedResult{Values: append([]provider.SeedValue(nil), request.Values...)}, nil
}

func (o *Provider) buildSeedRecords(request provider.SeedRequest) (map[string]map[string]string, error) {
	if len(request.Values) != len(o.fields) {
		return nil, fmt.Errorf("Valkey seed requires all %d fields", len(o.fields))
	}
	records := make(map[string]map[string]string, len(request.Values))
	seen := make(map[string]bool, len(request.Values))
	for _, value := range request.Values {
		scope, exists := o.fields[value.Field]
		if !exists || seen[value.Field] {
			return nil, fmt.Errorf("unknown or duplicate Valkey seed field %q", value.Field)
		}
		seen[value.Field] = true
		if !scope.Seed {
			continue
		}
		values, err := provider.Convert[map[string]string](value.Value)
		if err != nil {
			return nil, fmt.Errorf("Valkey seed %s: %w", value.Field, err)
		}
		concrete := make(map[string]string, len(values))
		for key, value := range values {
			concrete[scope.Prefix+key] = value
		}
		records[value.Field] = concrete
	}
	if err := o.validateOverlappingSeedValues(records); err != nil {
		return nil, err
	}
	return records, nil
}

func (o *Provider) validateOverlappingSeedValues(records map[string]map[string]string) error {
	for field, values := range records {
		for other, candidates := range records {
			if field == other {
				continue
			}
			for key, value := range candidates {
				if !strings.HasPrefix(key, o.fields[field].Prefix) {
					continue
				}
				expected, exists := values[key]
				if !exists || expected != value {
					return fmt.Errorf("Valkey seed fields %s and %s have conflicting values for key %q in overlapping keyspaces", field, other, key)
				}
			}
		}
	}
	return nil
}

func (o *Provider) clearSeedKeyspaces(ctx context.Context) error {
	var failures []error
	for field, scope := range o.fields {
		if !scope.Seed {
			continue
		}
		if err := o.deleteMatchingKeys(ctx, scope.Pattern); err != nil {
			failures = append(failures, fmt.Errorf("Valkey clear %s: %w", field, err))
		}
	}
	return errors.Join(failures...)
}

func (o *Provider) deleteMatchingKeys(ctx context.Context, pattern string) error {
	keys, err := o.scanKeys(ctx, pattern)
	if err != nil {
		return err
	}
	for start := 0; start < len(keys); start += mgetBatchSize {
		batch := keys[start:min(start+mgetBatchSize, len(keys))]
		if err := o.client.Do(ctx, o.client.B().Del().Key(batch...).Build()).Error(); err != nil {
			return err
		}
	}
	return nil
}
