//go:build specguardian

package specguardian_test

import (
	"context"
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
	e.clearRecords(t)
}

// BuildReadInput maps the abstract FizzBee input to the application's concrete argument.
func (e *ApplicationEnvironment) BuildReadInput(t *testing.T, input FizzbeeApplicationState) ApplicationReadInput {
	t.Helper()
	return &cacheaside.ReadInput{
		Key: input.Params.Application.Key,
	}
}

// BuildReadManualObservation combines storage state with the Read output and error.
func (e *ApplicationEnvironment) BuildReadManualObservation(t *testing.T, fetched ApplicationFetchedObservation, input FizzbeeApplicationState, output ApplicationReadOutput, actionErr error) ApplicationManualObservation {
	t.Helper()
	var resValue *string
	if output != nil {
		resValue = output.Value
	}

	return ApplicationManualObservation{
		Fields: ApplicationManualField{
			Application: ApplicationApplicationManualField{
				ReadResValue: gcmp.Observed(resValue),
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
	e.clearRecords(t)
}

// BuildWriteInput maps the abstract FizzBee input to the application's concrete argument.
func (e *ApplicationEnvironment) BuildWriteInput(t *testing.T, input FizzbeeApplicationState) ApplicationWriteInput {
	t.Helper()
	return &cacheaside.WriteInput{
		Key:   input.Params.Application.Key,
		Value: input.Params.Application.WriteValue,
	}
}

// BuildWriteManualObservation combines storage state with the Write output and error.
func (e *ApplicationEnvironment) BuildWriteManualObservation(t *testing.T, fetched ApplicationFetchedObservation, input FizzbeeApplicationState, output ApplicationWriteOutput, actionErr error) ApplicationManualObservation {
	t.Helper()

	return ApplicationManualObservation{
		Fields: ApplicationManualField{
			Application: ApplicationApplicationManualField{
				ReadResValue: gcmp.Unobservable[*string]("irrelevant to Write action"),
			},
		},
	}
}

func (e *ApplicationEnvironment) setupAttempt(t *testing.T, input FizzbeeApplicationState) {
	t.Helper()
	e.clearRecords(t)

	for key, value := range input.Fields.DB.Records {
		_, err := e.db.ExecContext(t.Context(), "INSERT INTO records(KEY, value) VALUES ($1, $2)", key, value)
		require.NoError(t, err)
	}
	for key, value := range input.Fields.Cache.CachedRecords {
		err := e.cache.Do(t.Context(), e.cache.B().Set().Key(key).Value(value).Build()).Error()
		require.NoError(t, err)
	}
}

func (e *ApplicationEnvironment) cleanupCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}

func (e *ApplicationEnvironment) clearRecords(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := e.db.ExecContext(ctx, "TRUNCATE TABLE records")
	require.NoError(t, err)
	require.NoError(t, e.cache.Do(ctx, e.cache.B().Flushdb().Build()).Error())
}
