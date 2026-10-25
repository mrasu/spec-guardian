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

// CacheAsideEnvironment holds the clients used by the application under test.
type CacheAsideEnvironment struct {
	db    *sql.DB
	cache valkey.Client
}

// NewCacheAsideEnvironment stores the clients used by one conformance case.
func NewCacheAsideEnvironment(db *sql.DB, cache valkey.Client) *CacheAsideEnvironment {
	if db == nil || cache == nil {
		panic("specguardian: nil application test resource")
	}
	return &CacheAsideEnvironment{db: db, cache: cache}
}

// BuildCacheAsideConformanceComponents constructs the action adapter and its test resources. Add SpecGuardian hooks to every dependency whose failures should be injected.
func BuildCacheAsideConformanceComponents(t *testing.T, controller *guardian.InjectionController) (CacheAsideActions, *CacheAsideEnvironment) {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	require.NoError(t, err)
	db := sql.OpenDB(gsql.NewConnector(stdlib.GetConnector(*config), controller))

	cache, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	require.NoError(t, err)
	cache = valkeyhook.WithHook(cache, gvalkeygo.NewHook(controller))

	return cacheaside.NewCacheAside(db, cache), NewCacheAsideEnvironment(db, cache)
}

// CacheAsideReadInput is the concrete application input for CacheAside.Read.
// Replace this scaffold with a type alias or local type as needed.
type CacheAsideReadInput = *cacheaside.ReadInput

// CacheAsideReadOutput is the concrete application output for CacheAside.Read.
// Replace this scaffold with a type alias or local type as needed.
type CacheAsideReadOutput = *cacheaside.ReadOutput

// CleanupReadCase cleans up after one conformance case.
func (e *CacheAsideEnvironment) CleanupReadCase(t *testing.T) {
	t.Helper()
	e.cleanupCase(t)
}

// BuildReadInput maps the abstract FizzBee input to the application's concrete argument.
func (e *CacheAsideEnvironment) BuildReadInput(t *testing.T, input FizzbeeCacheAsideState) CacheAsideReadInput {
	t.Helper()
	return &cacheaside.ReadInput{
		Key: input.Params.CacheAside.Key,
	}
}

// BuildReadManualObservation combines storage state with the Read output and error.
func (e *CacheAsideEnvironment) BuildReadManualObservation(t *testing.T, fetched CacheAsideFetchedObservation, input FizzbeeCacheAsideState, output CacheAsideReadOutput, actionErr error) CacheAsideManualObservation {
	t.Helper()
	var resValue *string
	if output != nil {
		resValue = output.Value
	}

	return CacheAsideManualObservation{
		Fields: CacheAsideManualField{
			CacheAside: CacheAsideCacheAsideManualField{
				ReadResValue: gcmp.Observed(resValue),
			},
		},
	}
}

// CacheAsideWriteAndInvalidateInput is the concrete application input for CacheAside.WriteAndInvalidate.
// Replace this scaffold with a type alias or local type as needed.
type CacheAsideWriteAndInvalidateInput = *cacheaside.WriteAndInvalidateInput

// CacheAsideWriteAndInvalidateOutput is the concrete application output for CacheAside.WriteAndInvalidate.
// Replace this scaffold with a type alias or local type as needed.
type CacheAsideWriteAndInvalidateOutput = *cacheaside.WriteAndInvalidateOutput

// CleanupWriteAndInvalidateCase cleans up after one conformance case.
func (e *CacheAsideEnvironment) CleanupWriteAndInvalidateCase(t *testing.T) {
	t.Helper()
	e.cleanupCase(t)
}

// BuildWriteAndInvalidateInput maps the abstract FizzBee input to the application's concrete argument.
func (e *CacheAsideEnvironment) BuildWriteAndInvalidateInput(t *testing.T, input FizzbeeCacheAsideState) CacheAsideWriteAndInvalidateInput {
	t.Helper()
	return &cacheaside.WriteAndInvalidateInput{
		Key:   input.Params.CacheAside.Key,
		Value: input.Params.CacheAside.WriteValue,
	}
}

// BuildWriteAndInvalidateManualObservation combines storage state with the WriteAndInvalidate output and error.
func (e *CacheAsideEnvironment) BuildWriteAndInvalidateManualObservation(t *testing.T, fetched CacheAsideFetchedObservation, input FizzbeeCacheAsideState, output CacheAsideWriteAndInvalidateOutput, actionErr error) CacheAsideManualObservation {
	t.Helper()

	return CacheAsideManualObservation{
		Fields: CacheAsideManualField{
			CacheAside: CacheAsideCacheAsideManualField{
				ReadResValue: gcmp.Unobservable[*string]("irrelevant to WriteAndInvalidate action"),
			},
		},
	}
}

func (e *CacheAsideEnvironment) cleanupCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}
