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

// CacheAsideEnvironment holds the database and cache used to prepare test cases.
type CacheAsideEnvironment struct {
	db    *sql.DB
	cache valkey.Client
}

// NewCacheAsideEnvironment stores the resources used to prepare a conformance case.
func NewCacheAsideEnvironment(db *sql.DB, cache valkey.Client) *CacheAsideEnvironment {
	if db == nil || cache == nil {
		panic("specguardian: nil application test resource")
	}
	return &CacheAsideEnvironment{db: db, cache: cache}
}

// BuildCacheAsideConformanceComponents connects fault injection to real I/O.
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

// CacheAsideWriteAndInvalidateInput is the concrete input for CacheAside.WriteAndInvalidate.
type CacheAsideWriteAndInvalidateInput = *cacheaside.WriteAndInvalidateInput

// CacheAsideWriteAndInvalidateOutput is the concrete output from CacheAside.WriteAndInvalidate.
type CacheAsideWriteAndInvalidateOutput = *cacheaside.WriteAndInvalidateOutput

// CleanupWriteAndInvalidateCase releases the clients after all attempts.
func (e *CacheAsideEnvironment) CleanupWriteAndInvalidateCase(t *testing.T) {
	t.Helper()
	e.cache.Close()
	require.NoError(t, e.db.Close())
}

// BuildWriteAndInvalidateInput maps the model's arguments to the real application.
func (e *CacheAsideEnvironment) BuildWriteAndInvalidateInput(t *testing.T, input FizzbeeCacheAsideState) CacheAsideWriteAndInvalidateInput {
	t.Helper()
	return &cacheaside.WriteAndInvalidateInput{Key: input.Params.CacheAside.Key, Value: input.Params.CacheAside.WriteValue}
}

// BuildWriteAndInvalidateManualObservation supplies fields not fetched by providers.
func (e *CacheAsideEnvironment) BuildWriteAndInvalidateManualObservation(t *testing.T, fetched CacheAsideFetchedObservation, input FizzbeeCacheAsideState, output CacheAsideWriteAndInvalidateOutput, actionErr error) CacheAsideManualObservation {
	t.Helper()
	return CacheAsideManualObservation{}
}
