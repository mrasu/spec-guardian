// Package provider defines the public boundary between generated tests and data providers.
package provider

import "context"

// Type describes the value prepared or observed for a FizzBee field.
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

// ActionCrashedField identifies whether the modeled Action was terminated by a crash.
const ActionCrashedField = "sg_metadata.action_crashed"

// ActionCrashedOutputSpec returns the fixed output shape without shared mutable state.
func ActionCrashedOutputSpec() OutputSpec {
	return OutputSpec{Type: Type{Kind: KindBool}}
}

// FieldRequest is one FizzBee field served by a provider.
type FieldRequest struct {
	Field  string
	Config map[string]any
	Output OutputSpec
}

// Lifecycle prepares and cleans up a provider for cases and attempts.
type Lifecycle interface {
	PrepareCase(context.Context) error
	PrepareAttempt(context.Context) error
	CleanupAttempt(context.Context) error
	CleanupCase(context.Context) error
}

// Observer observes fields independently of the Action result.
type Observer interface {
	Observe(ctx context.Context, field string) (any, error)
}

// ResultObserver observes fields using the Action output and error.
// Observation errors indicate a failure to observe, independently of actionErr.
type ResultObserver interface {
	ObserveResult(ctx context.Context, field string, actionOutput any, actionErr error) (any, error)
}

// Seeder prepares model input values and returns their concrete counterparts.
type Seeder interface {
	Seed(context.Context, SeedRequest) (SeedResult, error)
}

// SeedValue identifies one model field and its value.
type SeedValue struct {
	Field string
	Value any
}

// SeedRequest contains all fields assigned to a seeding provider, in order.
type SeedRequest struct {
	Values []SeedValue
}

// SeedResult preserves request field and list order. Observation-only fields
// pass through unchanged; their preparation remains the provider's responsibility.
type SeedResult struct {
	Values []SeedValue
}
