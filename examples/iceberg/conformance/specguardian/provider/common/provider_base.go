package common

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/provider"
)

// BaseProvider provides field registration and lifecycle management for Iceberg providers.
type BaseProvider struct {
	kind         string
	fields       map[string]struct{}
	tableObjects *TableObjects
	prepared     bool
}

// NewBaseProvider validates configuration and registers fields using the
// provider's type validation.
func NewBaseProvider(kind string, config map[string]any, fields []provider.FieldRequest, validateType func(provider.Type) error) (*BaseProvider, error) {
	if kind == "" || validateType == nil {
		return nil, errors.New("a provider kind and type validator are required")
	}
	if len(config) != 0 {
		return nil, errors.Newf("%s provider does not accept shared config", kind)
	}
	registered := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.Field == "" {
			return nil, errors.Newf("%s field ID is empty", kind)
		}
		if _, exists := registered[field.Field]; exists {
			return nil, errors.Newf("duplicate %s field %q", kind, field.Field)
		}
		if len(field.Config) != 0 {
			return nil, errors.Newf("%s field %s does not accept config", kind, field.Field)
		}
		if err := validateType(field.Output.Type); err != nil {
			return nil, err
		}
		registered[field.Field] = struct{}{}
	}
	return &BaseProvider{kind: kind, fields: registered}, nil
}

// SetTable supplies storage for the next attempt.
func (b *BaseProvider) SetTable(client *s3.Client, bucket, prefix string) error {
	objects, err := NewTableObjects(client, bucket, prefix)
	if err != nil {
		return err
	}
	b.tableObjects = objects
	return nil
}

// PrepareCase begins a conformance case.
func (b *BaseProvider) PrepareCase(context.Context) error { return nil }

// PrepareAttempt checks that setup supplied storage before observation.
func (b *BaseProvider) PrepareAttempt(context.Context) error {
	if b.tableObjects == nil {
		return errors.Newf("%s provider table is not set", b.kind)
	}
	b.prepared = true
	return nil
}

// CleanupAttempt clears storage and preparation state after an attempt.
func (b *BaseProvider) CleanupAttempt(context.Context) error {
	b.tableObjects = nil
	b.prepared = false
	return nil
}

// CleanupCase ends a conformance case.
func (b *BaseProvider) CleanupCase(context.Context) error { return nil }

// CheckField checks registration and preparation before reading a field.
func (b *BaseProvider) CheckField(field string) error {
	if _, ok := b.fields[field]; !ok {
		return errors.Newf("unknown %s field %q", b.kind, field)
	}
	if !b.prepared {
		return errors.Newf("%s provider attempt is not prepared", b.kind)
	}
	return nil
}

// TableObjects returns storage supplied by SetTable.
func (b *BaseProvider) TableObjects() *TableObjects {
	return b.tableObjects
}
