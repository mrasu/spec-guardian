//go:build specguardian

package specguardian_test

import (
	"database/sql"
	"testing"

	cacheaside "spec-guardian/examples/tutorial-cache-aside-write"

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

// ApplicationEnvironment holds the database and cache inspected by the test.
type ApplicationEnvironment struct {
	db    *sql.DB
	cache valkey.Client
}

// NewApplicationEnvironment stores the resources observed by a conformance case.
func NewApplicationEnvironment(db *sql.DB, cache valkey.Client) *ApplicationEnvironment {
	if db == nil || cache == nil {
		panic("specguardian: nil application test resource")
	}
	return &ApplicationEnvironment{db: db, cache: cache}
}

// BuildApplicationConformanceComponents connects fault injection to real I/O.
func BuildApplicationConformanceComponents(t *testing.T, controller *guardian.InjectionController) (ApplicationActions, *ApplicationEnvironment) {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	require.NoError(t, err)
	db := sql.OpenDB(gsql.NewConnector(stdlib.GetConnector(*config), controller))

	cache, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	require.NoError(t, err)
	cache = valkeyhook.WithHook(cache, gvalkeygo.NewHook(controller))

	return cacheaside.NewApplication(db, cache), NewApplicationEnvironment(db, cache)
}

// ObserveCurrentState reads the persistent state after an action.
func (e *ApplicationEnvironment) ObserveCurrentState(t *testing.T) ApplicationObservedState {
	t.Helper()
	return ApplicationObservedState{
		Fields: ApplicationObservedFields{
			DB:    ApplicationDBObservedFields{Records: gcmp.Observed(readDBRecords(t, e.db))},
			Cache: ApplicationCacheObservedFields{CachedRecords: gcmp.Observed(readCacheRecords(t, e.cache))},
		},
	}
}

// ApplicationWriteInput is the concrete input for Application.Write.
type ApplicationWriteInput = *cacheaside.WriteInput

// ApplicationWriteOutput is the concrete output from Application.Write.
type ApplicationWriteOutput = *cacheaside.WriteOutput

// SetupWriteCase prepares resources shared by attempts in one case.
func (e *ApplicationEnvironment) SetupWriteCase(t *testing.T) {
	t.Helper()
}

// CleanupWriteCase releases the clients after all attempts.
func (e *ApplicationEnvironment) CleanupWriteCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}

// SetupWriteAttempt restores the model's starting state before each fault.
func (e *ApplicationEnvironment) SetupWriteAttempt(t *testing.T, input FizzbeeApplicationState) {
	t.Helper()
	setupRecords(t, e.db, e.cache, input.Fields.DB.Records, input.Fields.Cache.CachedRecords)
}

// CleanupWriteAttempt clears state independently of the test context.
func (e *ApplicationEnvironment) CleanupWriteAttempt(t *testing.T) {
	t.Helper()
	clearRecords(t, e.db, e.cache)
}

// BuildWriteInput maps the model's arguments to the real application.
func (e *ApplicationEnvironment) BuildWriteInput(t *testing.T, input FizzbeeApplicationState) ApplicationWriteInput {
	t.Helper()
	return &cacheaside.WriteInput{Key: input.Params.Application.Key, Value: input.Params.Application.WriteValue}
}

// BuildObservedWriteState combines persistent state with the action result.
func (e *ApplicationEnvironment) BuildObservedWriteState(t *testing.T, observedState ApplicationObservedState, input FizzbeeApplicationState, output ApplicationWriteOutput, actionErr error) ApplicationObservedState {
	t.Helper()
	status := "DONE"
	if actionErr != nil {
		status = "FAILED"
	}
	return ApplicationObservedState{
		Params: ApplicationObservedParams{Application: ApplicationApplicationObservedParams{
			Key: gcmp.Observed(input.Params.Application.Key), WriteValue: gcmp.Observed(input.Params.Application.WriteValue),
		}},
		Fields: ApplicationObservedFields{
			DB:          observedState.Fields.DB,
			Cache:       observedState.Fields.Cache,
			Application: ApplicationApplicationObservedFields{WriteStatus: gcmp.Observed(status)},
		},
	}
}

// BuildAllowedWriteState compares visible fields with an allowed model state.
func (e *ApplicationEnvironment) BuildAllowedWriteState(t *testing.T, allowed FizzbeeApplicationState) ApplicationComparisonState {
	t.Helper()
	return ApplicationComparisonState{
		Params: ApplicationComparisonParams{Application: ApplicationApplicationComparisonParams{
			Key: gcmp.Equal(allowed.Params.Application.Key), WriteValue: gcmp.Equal(allowed.Params.Application.WriteValue),
		}},
		Fields: ApplicationComparisonFields{
			DB:          ApplicationDBComparisonFields{Records: gcmp.Equal(allowed.Fields.DB.Records)},
			Cache:       ApplicationCacheComparisonFields{CachedRecords: gcmp.Equal(allowed.Fields.Cache.CachedRecords)},
			Application: ApplicationApplicationComparisonFields{WriteStatus: gcmp.Equal(allowed.Fields.Application.WriteStatus)},
		},
	}
}
