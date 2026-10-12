//go:build specguardian

package specguardian_test

import (
	"context"
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

// BuildWriteManualObservation maps the action result to the model's write status.
func (e *ApplicationEnvironment) BuildWriteManualObservation(t *testing.T, fetched ApplicationFetchedObservation, input FizzbeeApplicationState, output ApplicationWriteOutput, actionErr error) ApplicationManualObservation {
	t.Helper()
	status := "DONE"
	if actionErr != nil {
		status = "FAILED"
	}
	return ApplicationManualObservation{
		Fields: ApplicationManualField{
			Application: ApplicationApplicationManualField{WriteStatus: gcmp.Observed(status)},
		},
	}
}

func setupRecords(t *testing.T, db *sql.DB, cache valkey.Client, dbRecords, cacheRecords map[string]string) {
	t.Helper()
	clearRecords(t, db, cache)

	for key, value := range dbRecords {
		_, err := db.ExecContext(t.Context(), "INSERT INTO records(KEY, value) VALUES ($1, $2)", key, value)
		require.NoError(t, err)
	}
	for key, value := range cacheRecords {
		err := cache.Do(t.Context(), cache.B().Set().Key(key).Value(value).Build()).Error()
		require.NoError(t, err)
	}
}

func clearRecords(t *testing.T, db *sql.DB, cache valkey.Client) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "TRUNCATE TABLE records")
	require.NoError(t, err)
	require.NoError(t, cache.Do(ctx, cache.B().Flushdb().Build()).Error())
}
