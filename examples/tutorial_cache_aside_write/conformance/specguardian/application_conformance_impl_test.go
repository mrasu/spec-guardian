//go:build specguardian

package specguardian_test

import (
	"database/sql"
	"testing"

	cacheaside "spec-guardian/examples/tutorial-cache-aside-write"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mrasu/spec-guardian/guardian"
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

// ApplicationEnvironment holds the database and cache used to prepare test cases.
type ApplicationEnvironment struct {
	db    *sql.DB
	cache valkey.Client
}

// NewApplicationEnvironment stores the resources used to prepare a conformance case.
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

// ApplicationWriteInput is the concrete input for Application.Write.
type ApplicationWriteInput = *cacheaside.WriteInput

// ApplicationWriteOutput is the concrete output from Application.Write.
type ApplicationWriteOutput = *cacheaside.WriteOutput

// CleanupWriteCase releases the clients after all attempts.
func (e *ApplicationEnvironment) CleanupWriteCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}

// BuildWriteInput maps the model's arguments to the real application.
func (e *ApplicationEnvironment) BuildWriteInput(t *testing.T, input FizzbeeApplicationState) ApplicationWriteInput {
	t.Helper()
	return &cacheaside.WriteInput{Key: input.Params.Application.Key, Value: input.Params.Application.WriteValue}
}

// BuildWriteManualObservation supplies fields not fetched by providers.
func (e *ApplicationEnvironment) BuildWriteManualObservation(t *testing.T, fetched ApplicationFetchedObservation, input FizzbeeApplicationState, output ApplicationWriteOutput, actionErr error) ApplicationManualObservation {
	t.Helper()
	return ApplicationManualObservation{}
}
