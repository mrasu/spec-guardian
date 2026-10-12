//go:build specguardian

package specguardian_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"uuid"

	"spec-guardian/examples/iceberg/conformance/specguardian/file"

	iceberggo "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	iceio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud"
	"github.com/apache/iceberg-go/table"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/stretchr/testify/require"
)

const (
	conformanceBucket          = "iceberg-warehouse"
	conformanceS3Endpoint      = "http://localhost:9000"
	conformanceCatalogEndpoint = "http://localhost:8181/api/catalog"
)

// IcebergTableEnvironment holds application-specific test resources. SpecGuardian generated this initial scaffold, but application owners may freely change its fields and helpers.
type IcebergTableEnvironment struct {
	adminConfig aws.Config
	catalog     *rest.Catalog
	s3          *s3.Client
	identifier  table.Identifier
	table       *table.Table
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

// SetupAppendAttempt prepares application state before IcebergTable.Append.
func (e *IcebergTableConformanceEnvironment) SetupAppendAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	e.setupObservedAttempt(t, input)
}

func (e *IcebergTableConformanceEnvironment) setupObservedAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	t.Helper()
	e.setupAttempt(t, input)
	prefix := strings.Join(e.identifier, "/") + "/"
	require.NoError(t, e.observerMetadataFilesObserver.SetTable(e.s3, conformanceBucket, prefix))
	require.NoError(t, e.observerManifestListsObserver.SetTable(e.s3, conformanceBucket, prefix))
	require.NoError(t, e.observerManifestFilesObserver.SetTable(e.s3, conformanceBucket, prefix))
	require.NoError(t, e.observerDataFilesObserver.SetTable(e.s3, conformanceBucket, prefix))
	require.NoError(t, e.observerDeleteFilesObserver.SetTable(e.s3, conformanceBucket, prefix))
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

// BuildAppendManualObservation returns fields not supplied by declared observers.
func (e *IcebergTableEnvironment) BuildAppendManualObservation(t *testing.T, fetched IcebergTableFetchedObservation, input FizzbeeIcebergTableState, output IcebergTableAppendOutput, actionErr error) IcebergTableManualObservation {
	t.Helper()
	return IcebergTableManualObservation{}
}

// IcebergTableOverwriteInput is the concrete application input for IcebergTable.Overwrite.
type IcebergTableOverwriteInput struct {
	Filter iceberggo.BooleanExpression
	Rows   []icebergRow
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

// SetupOverwriteAttempt prepares application state and observers before IcebergTable.Overwrite.
func (e *IcebergTableConformanceEnvironment) SetupOverwriteAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	e.setupObservedAttempt(t, input)
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
		Filter: operation.predicateExpression(t),
		Rows:   operation.updatedRows(t, input),
	}
}

// BuildOverwriteManualObservation returns fields not supplied by declared observers.
func (e *IcebergTableEnvironment) BuildOverwriteManualObservation(t *testing.T, fetched IcebergTableFetchedObservation, input FizzbeeIcebergTableState, output IcebergTableOverwriteOutput, actionErr error) IcebergTableManualObservation {
	t.Helper()
	return IcebergTableManualObservation{}
}

// IcebergTableDeleteInput is the concrete application input for IcebergTable.Delete.
type IcebergTableDeleteInput struct{ Filter iceberggo.BooleanExpression }

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

// SetupDeleteAttempt prepares application state before IcebergTable.Delete.
func (e *IcebergTableConformanceEnvironment) SetupDeleteAttempt(t *testing.T, input FizzbeeIcebergTableState) {
	e.setupObservedAttempt(t, input)
}

// CleanupDeleteAttempt removes state left by this attempt. It runs through t.Cleanup, so use a context independent of t.Context when needed.
func (e *IcebergTableEnvironment) CleanupDeleteAttempt(t *testing.T) {
	e.cleanupAttempt(t)
}

// BuildDeleteInput maps the abstract FizzBee input to the application's concrete argument.
func (e *IcebergTableEnvironment) BuildDeleteInput(t *testing.T, input FizzbeeIcebergTableState) IcebergTableDeleteInput {
	t.Helper()
	operation := NewModelOperation(t, input)
	return IcebergTableDeleteInput{Filter: operation.predicateExpression(t)}
}

// BuildDeleteManualObservation returns fields not supplied by declared observers.
func (e *IcebergTableEnvironment) BuildDeleteManualObservation(t *testing.T, fetched IcebergTableFetchedObservation, input FizzbeeIcebergTableState, output IcebergTableDeleteOutput, actionErr error) IcebergTableManualObservation {
	t.Helper()
	return IcebergTableManualObservation{}
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
	e.createTable(t)
	e.seedModelFiles(t, input)
}

func (e *IcebergTableEnvironment) createTable(t *testing.T) {
	t.Helper()
	namespace := fmt.Sprintf("specguardian_conformance_%s", uuid.New())
	e.identifier = catalog.ToIdentifier(namespace, "events")
	ctx := t.Context()
	require.NoError(t, e.catalog.CreateNamespace(ctx, catalog.ToIdentifier(namespace), nil))

	schema := file.IcebergSchema
	created, err := e.catalog.CreateTable(ctx, e.identifier, schema)
	require.NoError(t, err)
	e.table = created
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
