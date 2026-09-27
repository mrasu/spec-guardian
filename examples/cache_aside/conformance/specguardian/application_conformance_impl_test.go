//go:build specguardian

package specguardian_test

import (
	"database/sql"
	"testing"

	cacheaside "spec-guardian/examples/cache-aside"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gcmp"
	"github.com/mrasu/spec-guardian/guardian/gsql"
	"github.com/mrasu/spec-guardian/guardian/gvalkeygo"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeyhook"
)

const (
	databaseURL = "postgres://postgres@localhost:15432/postgres?sslmode=disable"
	valkeyAddr  = "localhost:16379"
)

// ApplicationEnvironment holds application-specific test resources. SpecGuardian generated this initial scaffold, but application owners may freely change its fields and helpers.
type ApplicationEnvironment struct {
	db    *sql.DB
	cache valkey.Client
}

// BuildApplicationConformanceComponents constructs the action adapter and its test resources. Add SpecGuardian hooks to every dependency whose failures should be injected.
func BuildApplicationConformanceComponents(t *testing.T, controller *guardian.InjectionController) (ApplicationActions, *ApplicationEnvironment) {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	require.NoError(t, err)
	db := sql.OpenDB(gsql.NewConnector(stdlib.GetConnector(*config), controller))

	cache, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	require.NoError(t, err)
	cache = valkeyhook.WithHook(cache, gvalkeygo.NewHook(controller))

	return cacheaside.NewApplication(db, cache), &ApplicationEnvironment{db: db, cache: cache}
}

// ObserveCurrentState captures the application state.
func (e *ApplicationEnvironment) ObserveCurrentState(t *testing.T) ApplicationObservedState {
	t.Helper()
	return ApplicationObservedState{
		Fields: ApplicationObservedFields{
			DB: ApplicationDBObservedFields{
				Records: gcmp.Observed(readDBRecords(t, e.db)),
			},
			Cache: ApplicationCacheObservedFields{
				CachedRecords: gcmp.Observed(readCacheRecords(t, e.cache)),
			},
		},
	}
}

// ApplicationReadInput is the concrete application input for Application.Read.
// Replace this scaffold with a type alias or local type as needed.
type ApplicationReadInput = *cacheaside.ReadInput

// ApplicationReadOutput is the concrete application output for Application.Read.
// Replace this scaffold with a type alias or local type as needed.
type ApplicationReadOutput = *cacheaside.ReadOutput

// SetupReadCase prepares resources and test settings for one conformance case.
func (e *ApplicationEnvironment) SetupReadCase(t *testing.T) {
	t.Helper()
}

// CleanupReadCase cleans up after one conformance case.
func (e *ApplicationEnvironment) CleanupReadCase(t *testing.T) {
	t.Helper()
	e.cleanupCase(t)
}

// SetupReadAttempt prepares application state before Application.Read.
func (e *ApplicationEnvironment) SetupReadAttempt(t *testing.T, input FizzbeeApplicationState) {
	t.Helper()
	e.setupAttempt(t, input)
}

// CleanupReadAttempt removes state left by this attempt. It runs through t.Cleanup, so use a context independent of t.Context when needed.
func (e *ApplicationEnvironment) CleanupReadAttempt(t *testing.T) {
	t.Helper()
	clearRecords(t, e.db, e.cache)
}

// BuildReadInput maps the abstract FizzBee input to the application's concrete argument.
func (e *ApplicationEnvironment) BuildReadInput(t *testing.T, input FizzbeeApplicationState) ApplicationReadInput {
	t.Helper()
	return &cacheaside.ReadInput{
		Key: input.Params.Application.Key,
	}
}

// BuildObservedReadState combines observed resources with the action input, output, and error.
func (e *ApplicationEnvironment) BuildObservedReadState(t *testing.T, observedState ApplicationObservedState, input FizzbeeApplicationState, output ApplicationReadOutput, actionErr error) ApplicationObservedState {
	t.Helper()
	var resValue *string
	if output != nil {
		resValue = output.Value
	}

	status := "DONE"
	if actionErr != nil {
		status = "FAILED"
	}

	return ApplicationObservedState{
		Params: ApplicationObservedParams{
			Application: ApplicationApplicationObservedParams{
				Key:        gcmp.Observed(input.Params.Application.Key),
				WriteValue: gcmp.Observed(input.Params.Application.WriteValue),
			},
		},
		Fields: ApplicationObservedFields{
			DB: ApplicationDBObservedFields{
				CommittedValues: gcmp.Unobservable[map[string]any]("commit history is not stored"),
				Records:         observedState.Fields.DB.Records,
			},
			Cache: ApplicationCacheObservedFields{
				CachedRecords: observedState.Fields.Cache.CachedRecords,
			},
			Application: ApplicationApplicationObservedFields{
				ReadResValue: gcmp.Observed(resValue),
				ReadStatus:   gcmp.Observed(status),
				WriteStatus:  gcmp.Unobservable[string]("irrelevant to Read action"),
			},
		},
	}
}

// BuildAllowedReadState maps an allowed model state to field comparisons.
func (e *ApplicationEnvironment) BuildAllowedReadState(t *testing.T, allowed FizzbeeApplicationState) ApplicationComparisonState {
	t.Helper()
	return ApplicationComparisonState{
		Params: ApplicationComparisonParams{
			Application: ApplicationApplicationComparisonParams{
				Key:        gcmp.Equal(allowed.Params.Application.Key),
				WriteValue: gcmp.Equal(allowed.Params.Application.WriteValue),
			},
		},
		Fields: ApplicationComparisonFields{
			DB: ApplicationDBComparisonFields{
				CommittedValues: gcmp.Ignore[map[string]any]("commit history is not stored"),
				Records:         gcmp.Equal(allowed.Fields.DB.Records),
			},
			Cache: ApplicationCacheComparisonFields{
				CachedRecords: gcmp.Equal(allowed.Fields.Cache.CachedRecords),
			},
			Application: ApplicationApplicationComparisonFields{
				ReadResValue: gcmp.Equal(allowed.Fields.Application.ReadResValue),
				ReadStatus:   gcmp.Equal(allowed.Fields.Application.ReadStatus),
				WriteStatus:  gcmp.Ignore[string]("irrelevant to Read action"),
			},
		},
	}
}

// ApplicationWriteInput is the concrete application input for Application.Write.
// Replace this scaffold with a type alias or local type as needed.
type ApplicationWriteInput = *cacheaside.WriteInput

// ApplicationWriteOutput is the concrete application output for Application.Write.
// Replace this scaffold with a type alias or local type as needed.
type ApplicationWriteOutput = *cacheaside.WriteOutput

// SetupWriteCase prepares resources and test settings for one conformance case.
func (e *ApplicationEnvironment) SetupWriteCase(t *testing.T) {
	t.Helper()
}

// CleanupWriteCase cleans up after one conformance case.
func (e *ApplicationEnvironment) CleanupWriteCase(t *testing.T) {
	t.Helper()
	e.cleanupCase(t)
}

// SetupWriteAttempt prepares application state before Application.Write.
func (e *ApplicationEnvironment) SetupWriteAttempt(t *testing.T, input FizzbeeApplicationState) {
	t.Helper()
	e.setupAttempt(t, input)
}

// CleanupWriteAttempt removes state left by this attempt. It runs through t.Cleanup, so use a context independent of t.Context when needed.
func (e *ApplicationEnvironment) CleanupWriteAttempt(t *testing.T) {
	t.Helper()
	clearRecords(t, e.db, e.cache)
}

// BuildWriteInput maps the abstract FizzBee input to the application's concrete argument.
func (e *ApplicationEnvironment) BuildWriteInput(t *testing.T, input FizzbeeApplicationState) ApplicationWriteInput {
	t.Helper()
	return &cacheaside.WriteInput{
		Key:   input.Params.Application.Key,
		Value: input.Params.Application.WriteValue,
	}
}

// BuildObservedWriteState combines observed resources with the action input, output, and error.
func (e *ApplicationEnvironment) BuildObservedWriteState(t *testing.T, observedState ApplicationObservedState, input FizzbeeApplicationState, output ApplicationWriteOutput, actionErr error) ApplicationObservedState {
	t.Helper()

	status := "DONE"
	if actionErr != nil {
		status = "FAILED"
	}

	return ApplicationObservedState{
		Params: ApplicationObservedParams{
			Application: ApplicationApplicationObservedParams{
				Key:        gcmp.Observed(input.Params.Application.Key),
				WriteValue: gcmp.Observed(input.Params.Application.WriteValue),
			},
		},
		Fields: ApplicationObservedFields{
			DB: ApplicationDBObservedFields{
				CommittedValues: gcmp.Unobservable[map[string]any]("commit history is not stored"),
				Records:         observedState.Fields.DB.Records,
			},
			Cache: ApplicationCacheObservedFields{
				CachedRecords: observedState.Fields.Cache.CachedRecords,
			},
			Application: ApplicationApplicationObservedFields{
				ReadResValue: gcmp.Unobservable[*string]("irrelevant to Write action"),
				ReadStatus:   gcmp.Unobservable[string]("irrelevant to Write action"),
				WriteStatus:  gcmp.Observed(status),
			},
		},
	}
}

// BuildAllowedWriteState maps an allowed model state to field comparisons.
func (e *ApplicationEnvironment) BuildAllowedWriteState(t *testing.T, allowed FizzbeeApplicationState) ApplicationComparisonState {
	t.Helper()
	return ApplicationComparisonState{
		Params: ApplicationComparisonParams{
			Application: ApplicationApplicationComparisonParams{
				Key:        gcmp.Equal(allowed.Params.Application.Key),
				WriteValue: gcmp.Equal(allowed.Params.Application.WriteValue),
			},
		},
		Fields: ApplicationComparisonFields{
			DB: ApplicationDBComparisonFields{
				CommittedValues: gcmp.Ignore[map[string]any]("commit history is not stored"),
				Records:         gcmp.Equal(allowed.Fields.DB.Records),
			},
			Cache: ApplicationCacheComparisonFields{
				CachedRecords: gcmp.Equal(allowed.Fields.Cache.CachedRecords),
			},
			Application: ApplicationApplicationComparisonFields{
				ReadResValue: gcmp.Ignore[*string]("irrelevant to Write action"),
				ReadStatus:   gcmp.Ignore[string]("irrelevant to Write action"),
				WriteStatus:  gcmp.Equal(allowed.Fields.Application.WriteStatus),
			},
		},
	}
}

func (e *ApplicationEnvironment) setupAttempt(t *testing.T, input FizzbeeApplicationState) {
	t.Helper()
	setupRecords(t, e.db, e.cache, input.Fields.DB.Records, input.Fields.Cache.CachedRecords)
}

func (e *ApplicationEnvironment) cleanupCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}
