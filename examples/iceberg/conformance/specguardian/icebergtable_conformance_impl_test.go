//go:build specguardian

package specguardian_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	iceberggo "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	iceio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud"
	"github.com/apache/iceberg-go/table"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gcmp"
	"github.com/stretchr/testify/require"
)

const (
	conformanceBucket          = "iceberg-warehouse"
	conformanceS3Endpoint      = "http://localhost:9000"
	conformanceCatalogEndpoint = "http://localhost:8181/api/catalog"
)

// IcebergTableEnvironment holds application-specific test resources. SpecGuardian generated this initial scaffold, but application owners may freely change its fields and helpers.
type IcebergTableEnvironment struct {
	adminConfig     aws.Config
	catalog         *rest.Catalog
	s3              *s3.Client
	identifier      table.Identifier
	table           *table.Table
	initial         FizzbeeIcebergTableState
	before          map[string]struct{}
	currentLocation string
}

// BuildIcebergTableConformanceComponents constructs the action adapter and its test resources. Add SpecGuardian hooks to every dependency whose failures should be injected.
func BuildIcebergTableConformanceComponents(t *testing.T, controller *guardian.InjectionController) (IcebergTableActions, *IcebergTableEnvironment) {
	t.Helper()
	configuration := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("test", "test", "")),
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
	}
	properties := iceberggo.Properties{
		iceio.S3Region:                 "us-east-1",
		iceio.S3EndpointURL:            conformanceS3Endpoint,
		iceio.S3AccessKeyID:            "test",
		iceio.S3SecretAccessKey:        "test",
		iceio.S3ForceVirtualAddressing: "false",
	}
	cat, err := rest.NewCatalog(t.Context(), "polaris", conformanceCatalogEndpoint,
		rest.WithCredential("test:test"),
		rest.WithScope("PRINCIPAL_ROLE:ALL"),
		rest.WithWarehouseLocation("iceberg"),
		rest.WithHeaders(map[string]string{"X-Iceberg-Access-Delegation": ""}),
		rest.WithAdditionalProps(properties),
	)
	require.NoError(t, err)
	client := s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(conformanceS3Endpoint)
		options.UsePathStyle = true
	})
	environment := NewIcebergTableEnvironment(configuration, cat, client)
	return NewIcebergActions(environment, controller), environment
}

// ObserveCurrentState captures the application state.
func (e *IcebergTableEnvironment) ObserveCurrentState(t *testing.T) IcebergTableObservedState {
	t.Helper()
	after := e.listObjectKeys(t)
	newObjects := e.readNewObjects(t, after)
	current, err := e.catalog.LoadTable(t.Context(), e.identifier)
	require.NoError(t, err)
	e.verifyS3References(t, current, after, newObjects)
	validateNewObjects(t, newObjects)
	e.currentLocation = current.MetadataLocation()
	files := scanDataFiles(t, current)
	return IcebergTableObservedState{
		Fields: IcebergTableObservedFields{
			ObjectStore: IcebergTableObjectStoreObservedFields{
				Objects: gcmp.Observed(map[string]any{"files": files}),
			},
			Catalog: IcebergTableCatalogObservedFields{
				MetadataLocation: gcmp.Observed(strconv.FormatBool(current.MetadataLocation() != e.table.MetadataLocation())),
			},
		},
	}
}

// IcebergTableAppendInput is the concrete application input for IcebergTable.Append.
type IcebergTableAppendInput struct{ Row icebergRow }

// IcebergTableAppendOutput is the concrete application output for IcebergTable.Append.
type IcebergTableAppendOutput struct{ Table *table.Table }

// SetupAppendCase prepares resources and test settings for one conformance case.
func (e *IcebergTableEnvironment) SetupAppendCase(t *testing.T) {
	t.Helper()
	t.Parallel()
}

// CleanupAppendCase cleans up after one conformance case.
func (e *IcebergTableEnvironment) CleanupAppendCase(t *testing.T) {
	t.Helper()
}

// SetupAppendAttempt prepares application state before IcebergTable.Append.
func (e *IcebergTableEnvironment) SetupAppendAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	e.setupAttempt(t, input)
}

// CleanupAppendAttempt removes state left by this attempt. It runs through t.Cleanup, so use a context independent of t.Context when needed.
func (e *IcebergTableEnvironment) CleanupAppendAttempt(t *testing.T) {
	e.cleanupAttempt(t)
}

// BuildAppendInput maps the abstract FizzBee input to the application's concrete argument.
func (e *IcebergTableEnvironment) BuildAppendInput(t *testing.T, input FizzbeeIcebergTableState) IcebergTableAppendInput {
	t.Helper()
	operation := NewModelOperation(t, input)
	return IcebergTableAppendInput{Row: operation.appendRow(t)}
}

// BuildObservedAppendState combines observed resources with the action input, output, and error.
func (e *IcebergTableEnvironment) BuildObservedAppendState(t *testing.T, observedState IcebergTableObservedState, input FizzbeeIcebergTableState, output IcebergTableAppendOutput, actionErr error) IcebergTableObservedState {
	t.Helper()
	e.verifyActionOutput(t, output.Table, actionErr)
	result := "COMMITTED"
	if actionErr != nil {
		result = "FAILED"
	}

	const ignored = "not observed by this action"
	return IcebergTableObservedState{
		Params: IcebergTableObservedParams{
			Writer1: IcebergTableWriter1ObservedParams{
				ID:         gcmp.Unobservable[int](ignored),
				Background: gcmp.Unobservable[bool](ignored),
			},
			Writer2: IcebergTableWriter2ObservedParams{
				ID:         gcmp.Unobservable[int](ignored),
				Background: gcmp.Unobservable[bool](ignored),
			},
			IcebergTable: IcebergTableIcebergTableObservedParams{
				OperationType: gcmp.Unobservable[string](ignored),
				Predicate:     gcmp.Unobservable[any](ignored),
				Row:           gcmp.Unobservable[[]any](ignored),
				SetValues:     gcmp.Unobservable[any](ignored),
				Writer:        gcmp.Unobservable[string](ignored),
			},
		},
		Fields: IcebergTableObservedFields{
			ObjectStore: IcebergTableObjectStoreObservedFields{
				Objects: observedState.Fields.ObjectStore.Objects,
			},
			Catalog: IcebergTableCatalogObservedFields{
				MetadataLocation: observedState.Fields.Catalog.MetadataLocation,
			},
			Writer1: IcebergTableWriter1ObservedFields{
				Op: gcmp.Unobservable[map[string]any](ignored),
			},
			Writer2: IcebergTableWriter2ObservedFields{
				Op: gcmp.Unobservable[map[string]any](ignored),
			},
			IcebergTable: IcebergTableIcebergTableObservedFields{
				Result: gcmp.Observed(result),
			},
		},
	}
}

// BuildAllowedAppendState maps an allowed model state to field comparisons.
func (e *IcebergTableEnvironment) BuildAllowedAppendState(t *testing.T, allowed FizzbeeIcebergTableState) IcebergTableComparisonState {
	t.Helper()
	const ignored = "not observed by this action"
	return IcebergTableComparisonState{
		Params: IcebergTableComparisonParams{
			Writer1: IcebergTableWriter1ComparisonParams{
				ID:         gcmp.Ignore[int](ignored),
				Background: gcmp.Ignore[bool](ignored),
			},
			Writer2: IcebergTableWriter2ComparisonParams{
				ID:         gcmp.Ignore[int](ignored),
				Background: gcmp.Ignore[bool](ignored),
			},
			IcebergTable: IcebergTableIcebergTableComparisonParams{
				OperationType: gcmp.Ignore[string](ignored),
				Predicate:     gcmp.Ignore[any](ignored),
				Row:           gcmp.Ignore[[]any](ignored),
				SetValues:     gcmp.Ignore[any](ignored),
				Writer:        gcmp.Ignore[string](ignored),
			},
		},
		Fields: IcebergTableComparisonFields{
			ObjectStore: IcebergTableObjectStoreComparisonFields{
				Objects: gcmp.Equal(map[string]any{"files": modelDataFiles(t, allowed)}),
			},
			Catalog: IcebergTableCatalogComparisonFields{
				MetadataLocation: gcmp.Equal(strconv.FormatBool(allowed.Fields.Catalog.MetadataLocation != e.initial.Fields.Catalog.MetadataLocation)),
			},
			Writer1: IcebergTableWriter1ComparisonFields{
				Op: gcmp.Ignore[map[string]any](ignored),
			},
			Writer2: IcebergTableWriter2ComparisonFields{
				Op: gcmp.Ignore[map[string]any](ignored),
			},
			IcebergTable: IcebergTableIcebergTableComparisonFields{
				Result: gcmp.Equal(allowed.Fields.IcebergTable.Result),
			},
		},
	}
}

// IcebergTableDeleteInput is the concrete application input for IcebergTable.Delete.
type IcebergTableDeleteInput struct{ Predicate iceberggo.BooleanExpression }

// IcebergTableDeleteOutput is the concrete application output for IcebergTable.Delete.
type IcebergTableDeleteOutput struct{ Table *table.Table }

// SetupDeleteCase prepares resources and test settings for one conformance case.
func (e *IcebergTableEnvironment) SetupDeleteCase(t *testing.T) {
	t.Helper()
	t.Parallel()
}

// CleanupDeleteCase cleans up after one conformance case.
func (e *IcebergTableEnvironment) CleanupDeleteCase(t *testing.T) {
	t.Helper()
}

// SetupDeleteAttempt prepares application state before IcebergTable.Delete.
func (e *IcebergTableEnvironment) SetupDeleteAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	e.setupAttempt(t, input)
}

// CleanupDeleteAttempt removes state left by this attempt. It runs through t.Cleanup, so use a context independent of t.Context when needed.
func (e *IcebergTableEnvironment) CleanupDeleteAttempt(t *testing.T) { e.cleanupAttempt(t) }

// BuildDeleteInput maps the abstract FizzBee input to the application's concrete argument.
func (e *IcebergTableEnvironment) BuildDeleteInput(t *testing.T, input FizzbeeIcebergTableState) IcebergTableDeleteInput {
	t.Helper()
	operation := NewModelOperation(t, input)
	return IcebergTableDeleteInput{Predicate: operation.predicateExpression(t)}
}

// BuildObservedDeleteState combines observed resources with the action input, output, and error.
func (e *IcebergTableEnvironment) BuildObservedDeleteState(t *testing.T, observedState IcebergTableObservedState, input FizzbeeIcebergTableState, output IcebergTableDeleteOutput, actionErr error) IcebergTableObservedState {
	t.Helper()
	e.verifyActionOutput(t, output.Table, actionErr)
	result := "COMMITTED"
	if actionErr != nil {
		result = "FAILED"
	}

	const ignored = "not observed by this action"
	return IcebergTableObservedState{
		Params: IcebergTableObservedParams{
			Writer1: IcebergTableWriter1ObservedParams{
				ID:         gcmp.Unobservable[int](ignored),
				Background: gcmp.Unobservable[bool](ignored),
			},
			Writer2: IcebergTableWriter2ObservedParams{
				ID:         gcmp.Unobservable[int](ignored),
				Background: gcmp.Unobservable[bool](ignored),
			},
			IcebergTable: IcebergTableIcebergTableObservedParams{
				OperationType: gcmp.Unobservable[string](ignored),
				Predicate:     gcmp.Unobservable[any](ignored),
				Row:           gcmp.Unobservable[[]any](ignored),
				SetValues:     gcmp.Unobservable[any](ignored),
				Writer:        gcmp.Unobservable[string](ignored),
			},
		},
		Fields: IcebergTableObservedFields{
			ObjectStore: IcebergTableObjectStoreObservedFields{
				Objects: observedState.Fields.ObjectStore.Objects,
			},
			Catalog: IcebergTableCatalogObservedFields{
				MetadataLocation: observedState.Fields.Catalog.MetadataLocation,
			},
			Writer1: IcebergTableWriter1ObservedFields{
				Op: gcmp.Unobservable[map[string]any](ignored),
			},
			Writer2: IcebergTableWriter2ObservedFields{
				Op: gcmp.Unobservable[map[string]any](ignored),
			},
			IcebergTable: IcebergTableIcebergTableObservedFields{
				Result: gcmp.Observed(result),
			},
		},
	}
}

// BuildAllowedDeleteState maps an allowed model state to field comparisons.
func (e *IcebergTableEnvironment) BuildAllowedDeleteState(t *testing.T, allowed FizzbeeIcebergTableState) IcebergTableComparisonState {
	t.Helper()
	const ignored = "not observed by this action"
	return IcebergTableComparisonState{
		Params: IcebergTableComparisonParams{
			Writer1: IcebergTableWriter1ComparisonParams{
				ID:         gcmp.Ignore[int](ignored),
				Background: gcmp.Ignore[bool](ignored),
			},
			Writer2: IcebergTableWriter2ComparisonParams{
				ID:         gcmp.Ignore[int](ignored),
				Background: gcmp.Ignore[bool](ignored),
			},
			IcebergTable: IcebergTableIcebergTableComparisonParams{
				OperationType: gcmp.Ignore[string](ignored),
				Predicate:     gcmp.Ignore[any](ignored),
				Row:           gcmp.Ignore[[]any](ignored),
				SetValues:     gcmp.Ignore[any](ignored),
				Writer:        gcmp.Ignore[string](ignored),
			},
		},
		Fields: IcebergTableComparisonFields{
			ObjectStore: IcebergTableObjectStoreComparisonFields{
				Objects: gcmp.Equal(map[string]any{"files": modelDataFiles(t, allowed)}),
			},
			Catalog: IcebergTableCatalogComparisonFields{
				MetadataLocation: gcmp.Equal(strconv.FormatBool(allowed.Fields.Catalog.MetadataLocation != e.initial.Fields.Catalog.MetadataLocation)),
			},
			Writer1: IcebergTableWriter1ComparisonFields{
				Op: gcmp.Ignore[map[string]any](ignored),
			},
			Writer2: IcebergTableWriter2ComparisonFields{
				Op: gcmp.Ignore[map[string]any](ignored),
			},
			IcebergTable: IcebergTableIcebergTableComparisonFields{
				Result: gcmp.Equal(allowed.Fields.IcebergTable.Result),
			},
		},
	}
}

// IcebergTableOverwriteInput is the concrete application input for IcebergTable.Overwrite.
type IcebergTableOverwriteInput struct {
	Rows      []icebergRow
	Predicate iceberggo.BooleanExpression
}

// IcebergTableOverwriteOutput is the concrete application output for IcebergTable.Overwrite.
type IcebergTableOverwriteOutput struct{ Table *table.Table }

// SetupOverwriteCase prepares resources and test settings for one conformance case.
func (e *IcebergTableEnvironment) SetupOverwriteCase(t *testing.T) {
	t.Helper()
	t.Parallel()
}

// CleanupOverwriteCase cleans up after one conformance case.
func (e *IcebergTableEnvironment) CleanupOverwriteCase(t *testing.T) {
	t.Helper()
}

// SetupOverwriteAttempt prepares application state before IcebergTable.Overwrite.
func (e *IcebergTableEnvironment) SetupOverwriteAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	e.setupAttempt(t, input)
}

// CleanupOverwriteAttempt removes state left by this attempt. It runs through t.Cleanup, so use a context independent of t.Context when needed.
func (e *IcebergTableEnvironment) CleanupOverwriteAttempt(t *testing.T) {
	e.cleanupAttempt(t)
}

// BuildOverwriteInput maps the abstract FizzBee input to the application's concrete argument.
func (e *IcebergTableEnvironment) BuildOverwriteInput(t *testing.T, input FizzbeeIcebergTableState) IcebergTableOverwriteInput {
	t.Helper()
	operation := NewModelOperation(t, input)
	return IcebergTableOverwriteInput{
		Rows:      operation.updatedRows(t, input),
		Predicate: operation.predicateExpression(t),
	}
}

// BuildObservedOverwriteState combines observed resources with the action input, output, and error.
func (e *IcebergTableEnvironment) BuildObservedOverwriteState(t *testing.T, observedState IcebergTableObservedState, input FizzbeeIcebergTableState, output IcebergTableOverwriteOutput, actionErr error) IcebergTableObservedState {
	t.Helper()
	e.verifyActionOutput(t, output.Table, actionErr)
	result := "COMMITTED"
	if actionErr != nil {
		result = "FAILED"
	}

	const ignored = "not observed by this action"
	return IcebergTableObservedState{
		Params: IcebergTableObservedParams{
			Writer1: IcebergTableWriter1ObservedParams{
				ID:         gcmp.Unobservable[int](ignored),
				Background: gcmp.Unobservable[bool](ignored),
			},
			Writer2: IcebergTableWriter2ObservedParams{
				ID:         gcmp.Unobservable[int](ignored),
				Background: gcmp.Unobservable[bool](ignored),
			},
			IcebergTable: IcebergTableIcebergTableObservedParams{
				OperationType: gcmp.Unobservable[string](ignored),
				Predicate:     gcmp.Unobservable[any](ignored),
				Row:           gcmp.Unobservable[[]any](ignored),
				SetValues:     gcmp.Unobservable[any](ignored),
				Writer:        gcmp.Unobservable[string](ignored),
			},
		},
		Fields: IcebergTableObservedFields{
			ObjectStore: IcebergTableObjectStoreObservedFields{
				Objects: observedState.Fields.ObjectStore.Objects,
			},
			Catalog: IcebergTableCatalogObservedFields{
				MetadataLocation: observedState.Fields.Catalog.MetadataLocation,
			},
			Writer1: IcebergTableWriter1ObservedFields{
				Op: gcmp.Unobservable[map[string]any](ignored),
			},
			Writer2: IcebergTableWriter2ObservedFields{
				Op: gcmp.Unobservable[map[string]any](ignored),
			},
			IcebergTable: IcebergTableIcebergTableObservedFields{
				Result: gcmp.Observed(result),
			},
		},
	}
}

// BuildAllowedOverwriteState maps an allowed model state to field comparisons.
func (e *IcebergTableEnvironment) BuildAllowedOverwriteState(t *testing.T, allowed FizzbeeIcebergTableState) IcebergTableComparisonState {
	t.Helper()
	const ignored = "not observed by this action"
	return IcebergTableComparisonState{
		Params: IcebergTableComparisonParams{
			Writer1: IcebergTableWriter1ComparisonParams{
				ID:         gcmp.Ignore[int](ignored),
				Background: gcmp.Ignore[bool](ignored),
			},
			Writer2: IcebergTableWriter2ComparisonParams{
				ID:         gcmp.Ignore[int](ignored),
				Background: gcmp.Ignore[bool](ignored),
			},
			IcebergTable: IcebergTableIcebergTableComparisonParams{
				OperationType: gcmp.Ignore[string](ignored),
				Predicate:     gcmp.Ignore[any](ignored),
				Row:           gcmp.Ignore[[]any](ignored),
				SetValues:     gcmp.Ignore[any](ignored),
				Writer:        gcmp.Ignore[string](ignored),
			},
		},
		Fields: IcebergTableComparisonFields{
			ObjectStore: IcebergTableObjectStoreComparisonFields{
				Objects: gcmp.Equal(map[string]any{"files": modelDataFiles(t, allowed)}),
			},
			Catalog: IcebergTableCatalogComparisonFields{
				MetadataLocation: gcmp.Equal(strconv.FormatBool(allowed.Fields.Catalog.MetadataLocation != e.initial.Fields.Catalog.MetadataLocation)),
			},
			Writer1: IcebergTableWriter1ComparisonFields{
				Op: gcmp.Ignore[map[string]any](ignored),
			},
			Writer2: IcebergTableWriter2ComparisonFields{
				Op: gcmp.Ignore[map[string]any](ignored),
			},
			IcebergTable: IcebergTableIcebergTableComparisonFields{
				Result: gcmp.Equal(allowed.Fields.IcebergTable.Result),
			},
		},
	}
}

// NewIcebergTableEnvironment binds the catalog and storage clients used by the test.
func NewIcebergTableEnvironment(configuration aws.Config, cat *rest.Catalog, client *s3.Client) *IcebergTableEnvironment {
	if cat == nil || client == nil {
		panic("IcebergTableEnvironment requires catalog and S3 clients")
	}

	return &IcebergTableEnvironment{
		adminConfig: configuration,
		catalog:     cat,
		s3:          client,
	}
}

func (e *IcebergTableEnvironment) setupAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	t.Helper()
	e.initial = input
	e.createTable(t)
	e.seedCommittedFiles(t, input)
	e.before = e.listObjectKeys(t)
}

func (e *IcebergTableEnvironment) createTable(t *testing.T) {
	t.Helper()
	namespaceID := make([]byte, 16)
	_, err := rand.Read(namespaceID)
	require.NoError(t, err)

	namespace := fmt.Sprintf("specguardian_conformance_%s", uuid.New())
	e.identifier = catalog.ToIdentifier(namespace, "events")
	ctx := t.Context()
	require.NoError(t, e.catalog.CreateNamespace(ctx, catalog.ToIdentifier(namespace), nil))

	schema := iceberggo.NewSchema(0,
		iceberggo.NestedField{ID: 1, Name: "col1", Type: iceberggo.PrimitiveTypes.String, Required: true},
		iceberggo.NestedField{ID: 2, Name: "col2", Type: iceberggo.PrimitiveTypes.String, Required: true},
		iceberggo.NestedField{ID: 3, Name: "col3", Type: iceberggo.PrimitiveTypes.String, Required: true},
	)
	created, err := e.catalog.CreateTable(ctx, e.identifier, schema)
	require.NoError(t, err)
	e.table = created
}

func (e *IcebergTableEnvironment) seedCommittedFiles(t *testing.T, input FizzbeeIcebergTableState) {
	t.Helper()
	for _, rows := range modelLiveFiles(t, input) {
		reader, err := newActionRecords(e.table, rows)
		require.NoError(t, err)
		updated, err := e.table.Append(t.Context(), reader, nil)
		reader.Release()
		require.NoError(t, err)
		e.table = updated
	}
}

func (e *IcebergTableEnvironment) cleanupAttempt(t *testing.T) {
	t.Helper()
	if e.identifier == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, e.catalog.DropTable(ctx, e.identifier))
	require.NoError(t, e.catalog.DropNamespace(ctx, e.identifier[:1]))
	e.deleteObjects(t, ctx)
	e.identifier = nil
}

func (e *IcebergTableEnvironment) deleteObjects(t *testing.T, ctx context.Context) {
	t.Helper()
	prefix := strings.Join(e.identifier, "/") + "/"
	paginator := s3.NewListObjectsV2Paginator(e.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(conformanceBucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		require.NoError(t, err)
		for _, object := range page.Contents {
			_, err := e.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(conformanceBucket),
				Key:    object.Key,
			})
			require.NoError(t, err)
		}
	}
}

func (e *IcebergTableEnvironment) verifyActionOutput(t *testing.T, output *table.Table, actionErr error) {
	t.Helper()
	if actionErr != nil {
		require.Nil(t, output, "failed action returned a table")
		return
	}
	require.NotNil(t, output, "successful action returned no table")
	require.Equal(t, e.currentLocation, output.MetadataLocation(), "returned table differs from catalog")
}

func (e *IcebergTableEnvironment) listObjectKeys(t *testing.T) map[string]struct{} {
	t.Helper()
	prefix := strings.Join(e.identifier, "/") + "/"
	keys := make(map[string]struct{})
	paginator := s3.NewListObjectsV2Paginator(e.s3, &s3.ListObjectsV2Input{Bucket: aws.String(conformanceBucket), Prefix: aws.String(prefix)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(t.Context())
		require.NoError(t, err)
		for _, object := range page.Contents {
			keys[aws.ToString(object.Key)] = struct{}{}
		}
	}
	return keys
}

func (e *IcebergTableEnvironment) readNewObjects(t *testing.T, after map[string]struct{}) map[string][]byte {
	t.Helper()
	var keys []string
	for key := range after {
		if _, existed := e.before[key]; !existed {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)

	const maxConcurrentReads = 8
	semaphore := make(chan struct{}, maxConcurrentReads)
	contents := make([][]byte, len(keys))
	readErrors := make([]error, len(keys))
	var readers sync.WaitGroup
	ctx := t.Context()
	for index, key := range keys {
		semaphore <- struct{}{}
		readers.Add(1)
		go func() {
			defer readers.Done()
			defer func() { <-semaphore }()
			contents[index], readErrors[index] = e.readObject(ctx, key)
		}()
	}
	readers.Wait()

	objects := make(map[string][]byte, len(keys))
	for index, key := range keys {
		require.NoError(t, readErrors[index])
		objects[key] = contents[index]
	}
	return objects
}

func (e *IcebergTableEnvironment) readObject(ctx context.Context, key string) ([]byte, error) {
	response, err := e.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(conformanceBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "get S3 object %s", key)
	}
	contents, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err != nil {
		return nil, errors.Wrapf(err, "read S3 object %s", key)
	}
	if closeErr != nil {
		return nil, errors.Wrapf(closeErr, "close S3 object %s", key)
	}
	return contents, nil
}

func (e *IcebergTableEnvironment) verifyS3References(t *testing.T, current *table.Table, keys map[string]struct{}, newObjects map[string][]byte) {
	t.Helper()
	metadataKey := e.objectKey(t, current.MetadataLocation())
	_, found := keys[metadataKey]
	require.True(t, found, "catalog metadata %s is missing from S3", metadataKey)
	e.verifySnapshotReferences(t, current.CurrentSnapshot(), keys, newObjects)
}

func (e *IcebergTableEnvironment) verifySnapshotReferences(t *testing.T, snapshot *table.Snapshot, keys map[string]struct{}, newObjects map[string][]byte) {
	t.Helper()
	if snapshot == nil {
		return
	}
	listKey := e.objectKey(t, snapshot.ManifestList)
	_, found := keys[listKey]
	require.True(t, found, "manifest list %s is missing from S3", listKey)
	list := e.readReferencedObject(t, listKey, newObjects)
	manifests, err := iceberggo.ReadManifestList(bytes.NewReader(list))
	require.NoError(t, err)
	for _, manifest := range manifests {
		key := e.objectKey(t, manifest.FilePath())
		_, found := keys[key]
		require.True(t, found, "manifest %s is missing from S3", key)
		contents := e.readReferencedObject(t, key, newObjects)
		e.verifyManifestReferences(t, manifest, contents, keys)
	}
}

func (e *IcebergTableEnvironment) readReferencedObject(t *testing.T, key string, newObjects map[string][]byte) []byte {
	t.Helper()
	if contents, exists := newObjects[key]; exists {
		return contents
	}
	contents, err := e.readObject(t.Context(), key)
	require.NoError(t, err)
	return contents
}

func (e *IcebergTableEnvironment) verifyManifestReferences(t *testing.T, manifest iceberggo.ManifestFile, contents []byte, keys map[string]struct{}) {
	t.Helper()
	entries, err := iceberggo.ReadManifest(manifest, bytes.NewReader(contents), false)
	require.NoError(t, err)
	for _, entry := range entries {
		dataKey := e.objectKey(t, entry.DataFile().FilePath())
		_, found := keys[dataKey]
		require.True(t, found, "data file %s is missing from S3", dataKey)
	}
}

func (e *IcebergTableEnvironment) objectKey(t *testing.T, location string) string {
	t.Helper()
	prefix := "s3://" + conformanceBucket + "/"
	require.True(t, strings.HasPrefix(location, prefix), "unexpected S3 location %s", location)
	return strings.TrimPrefix(location, prefix)
}
