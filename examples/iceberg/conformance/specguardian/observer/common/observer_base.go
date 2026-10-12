package common

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/observer"
)

// BaseObserver provides field registration and lifecycle management for Iceberg observers.
type BaseObserver struct {
	kind         string
	fields       map[string]struct{}
	tableObjects *TableObjects
	prepared     bool
}

// NewBaseObserver validates configuration and registers fields using the
// observer's type validation.
func NewBaseObserver(kind string, config map[string]any, fields []observer.FieldRequest, validateType func(observer.Type) error) (*BaseObserver, error) {
	if kind == "" || validateType == nil {
		return nil, errors.New("an observer kind and type validator are required")
	}
	if len(config) != 0 {
		return nil, errors.Newf("%s observer does not accept shared config", kind)
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
	return &BaseObserver{kind: kind, fields: registered}, nil
}

// SetTable supplies storage for the next attempt.
func (b *BaseObserver) SetTable(client *s3.Client, bucket, prefix string) error {
	objects, err := NewTableObjects(client, bucket, prefix)
	if err != nil {
		return err
	}
	b.tableObjects = objects
	return nil
}

// PrepareCase begins a conformance case.
func (b *BaseObserver) PrepareCase(context.Context) error { return nil }

// PrepareAttempt checks that setup supplied storage before observation.
func (b *BaseObserver) PrepareAttempt(context.Context) error {
	if b.tableObjects == nil {
		return errors.Newf("%s observer table is not set", b.kind)
	}
	b.prepared = true
	return nil
}

// CleanupAttempt clears storage and preparation state after an attempt.
func (b *BaseObserver) CleanupAttempt(context.Context) error {
	b.tableObjects = nil
	b.prepared = false
	return nil
}

// CleanupCase ends a conformance case.
func (b *BaseObserver) CleanupCase(context.Context) error { return nil }

// CheckField checks registration and preparation before reading a field.
func (b *BaseObserver) CheckField(field string) error {
	if _, ok := b.fields[field]; !ok {
		return errors.Newf("unknown %s field %q", b.kind, field)
	}
	if !b.prepared {
		return errors.Newf("%s observer attempt is not prepared", b.kind)
	}
	return nil
}

// TableObjects returns storage supplied by SetTable.
func (b *BaseObserver) TableObjects() *TableObjects {
	return b.tableObjects
}
