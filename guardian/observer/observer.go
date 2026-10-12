// Package observer defines the public boundary between generated tests and observation plugins.
package observer

import "context"

// Type describes the value returned for a FizzBee field.
type Type struct {
	Kind     Kind
	Nullable bool
	Values   *Type
	Fields   map[string]Type
}

// Kind identifies the shape of an observed value.
type Kind string

const (
	KindMap     Kind = "map"
	KindList    Kind = "list"
	KindMessage Kind = "message"

	KindString Kind = "string"
	KindInt64  Kind = "int64"
	KindBool   Kind = "bool"
	KindDouble Kind = "double"
	KindBytes  Kind = "bytes"
)

// OutputSpec describes the output of a field observation.
type OutputSpec struct {
	Type Type
}

// FieldRequest is one FizzBee field served by an observer.
type FieldRequest struct {
	Field  string
	Config map[string]any
	Output OutputSpec
}

// Observer observes fields for one conformance case.
type Observer interface {
	PrepareCase(context.Context) error
	PrepareAttempt(context.Context) error
	Observe(ctx context.Context, field string) (any, error)
	CleanupAttempt(context.Context) error
	CleanupCase(context.Context) error
}
