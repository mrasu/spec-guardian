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

// ApplicationEnvironment holds the clients used by the application under test.
type ApplicationEnvironment struct {
	db    *sql.DB
	cache valkey.Client
}

// NewApplicationEnvironment stores the clients used by one conformance case.
func NewApplicationEnvironment(db *sql.DB, cache valkey.Client) *ApplicationEnvironment {
	if db == nil || cache == nil {
		panic("specguardian: nil application test resource")
	}
	return &ApplicationEnvironment{db: db, cache: cache}
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

	return cacheaside.NewApplication(db, cache), NewApplicationEnvironment(db, cache)
}

// ApplicationReadInput is the concrete application input for Application.Read.
// Replace this scaffold with a type alias or local type as needed.
type ApplicationReadInput = *cacheaside.ReadInput

// ApplicationReadOutput is the concrete application output for Application.Read.
// Replace this scaffold with a type alias or local type as needed.
type ApplicationReadOutput = *cacheaside.ReadOutput

// CleanupReadCase cleans up after one conformance case.
func (e *ApplicationEnvironment) CleanupReadCase(t *testing.T) {
	t.Helper()
	e.cleanupCase(t)
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

// CleanupWriteCase cleans up after one conformance case.
func (e *ApplicationEnvironment) CleanupWriteCase(t *testing.T) {
	t.Helper()
	e.cleanupCase(t)
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

func (e *ApplicationEnvironment) cleanupCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}
