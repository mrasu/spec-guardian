package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	iceio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud"
	"github.com/apache/iceberg-go/table"
	iceutils "github.com/apache/iceberg-go/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/guardian/gawssdkgov2"
)

const (
	bucket          = "iceberg-warehouse"
	polarisEndpoint = "http://localhost:8181/api/catalog"
	rustFSEndpoint  = "http://localhost:9000"
	rustFSConsole   = "http://localhost:9001"
	rustFSAccessKey = "test"
	rustFSSecretKey = "test"
	region          = "us-east-1"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	adminConfig, err := newAWSConfig(ctx)
	if err != nil {
		return err
	}
	cat, err := newCatalog(ctx)
	if err != nil {
		return err
	}
	identifier, tbl, err := createTable(ctx, cat)
	if err != nil {
		return err
	}
	fmt.Printf("Created Iceberg table %s in Polaris with columns id:int64 and name:string\n", strings.Join(identifier, "."))
	tablePrefix := strings.Join(identifier, "/") + "/"
	rows := []event{
		{id: 1, name: "alpha"},
		{id: 2, name: "beta"},
	}

	if err := appendRows(ctx, adminConfig, tbl, rows); err != nil {
		return err
	}

	if err := reloadAndInspect(ctx, adminConfig, cat, identifier, tablePrefix, len(rows)); err != nil {
		return err
	}
	fmt.Println("Iceberg validation completed: append, reload, scan, and object parsing succeeded")
	printRustFSInstructions(tablePrefix)
	return nil
}

func printRustFSInstructions(tablePrefix string) {
	fmt.Println()
	fmt.Println("👉 Inspect the generated Iceberg files in RustFS")
	fmt.Println()
	fmt.Printf("   Console:  %s\n", rustFSConsole)
	fmt.Printf("   Username: %s\n", rustFSAccessKey)
	fmt.Printf("   Password: %s\n", rustFSSecretKey)
	fmt.Printf("   Bucket:   %s\n", bucket)
	fmt.Printf("   Path:     %s\n", tablePrefix)
}

func newAWSConfig(ctx context.Context) (aws.Config, error) {
	options := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(rustFSAccessKey, rustFSSecretKey, "")),
		config.WithRetryer(func() aws.Retryer { return aws.NopRetryer{} }),
	}
	configuration, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return aws.Config{}, errors.Wrap(err, "load AWS configuration")
	}
	return configuration, nil
}

func appendRows(ctx context.Context, adminConfig aws.Config, tbl *table.Table, rows []event) error {
	trace := NewIOTrace()
	actionCtx := newActionContext(ctx, adminConfig, trace)
	fmt.Printf("Appending %d rows: %s\n", len(rows), formatRows(rows))

	if err := appendRecords(actionCtx, tbl, rows); err != nil {
		return err
	}
	fmt.Printf("Appended %d rows; S3 requests: %s\n", len(rows), strings.Join(trace.Operations(), ", "))
	return nil
}

func formatRows(rows []event) string {
	formatted := make([]string, 0, len(rows))
	for _, row := range rows {
		formatted = append(formatted, fmt.Sprintf("(%d, %s)", row.id, row.name))
	}
	return strings.Join(formatted, ", ")
}

func newActionContext(ctx context.Context, adminConfig aws.Config, trace *IOTrace) context.Context {
	actionConfig := adminConfig.Copy()
	interceptors := smithyhttp.InterceptorRegistry{}
	interceptors.AddBeforeExecution(gawssdkgov2.NewInterceptor(trace))
	actionConfig.Interceptors = interceptors
	return iceutils.WithAwsConfig(ctx, &actionConfig)
}

func appendRecords(ctx context.Context, tbl *table.Table, rows []event) error {
	records, err := newRecords(tbl.Schema(), rows)
	if err != nil {
		return err
	}
	defer records.Release()
	if _, err := tbl.Append(ctx, records, nil); err != nil {
		return errors.Wrap(err, "append records")
	}
	return nil
}

func reloadAndInspect(ctx context.Context, adminConfig aws.Config, cat catalog.Catalog, identifier table.Identifier, tablePrefix string, rowCount int) error {
	reloaded, err := cat.LoadTable(ctx, identifier)
	if err != nil {
		return errors.Wrap(err, "reload table")
	}
	fmt.Println("Reloaded Iceberg table from Polaris")
	if err := verifyRows(ctx, reloaded, int64(rowCount)); err != nil {
		return err
	}
	fmt.Printf("Scanned the reloaded table and verified all %d rows\n", rowCount)
	fmt.Printf("Inspecting Iceberg files in s3://%s/%s\n", bucket, tablePrefix)
	return inspectObjects(ctx, newS3Client(adminConfig), reloaded, tablePrefix)
}

func newS3Client(configuration aws.Config) *s3.Client {
	return s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(rustFSEndpoint)
		options.UsePathStyle = true
	})
}

func newCatalog(ctx context.Context) (*rest.Catalog, error) {
	properties := iceberg.Properties{
		iceio.S3Region:                 region,
		iceio.S3EndpointURL:            rustFSEndpoint,
		iceio.S3AccessKeyID:            rustFSAccessKey,
		iceio.S3SecretAccessKey:        rustFSSecretKey,
		iceio.S3ForceVirtualAddressing: "false",
	}
	cat, err := rest.NewCatalog(ctx, "polaris", polarisEndpoint,
		rest.WithCredential("test:test"),
		rest.WithScope("PRINCIPAL_ROLE:ALL"),
		rest.WithWarehouseLocation("iceberg"),
		rest.WithHeaders(map[string]string{"X-Iceberg-Access-Delegation": ""}),
		rest.WithAdditionalProps(properties),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create Polaris REST catalog")
	}
	return cat, nil
}

func createTable(ctx context.Context, cat catalog.Catalog) (table.Identifier, *table.Table, error) {
	namespace := fmt.Sprintf("specguardian_test_%d", time.Now().UnixNano())
	identifier := catalog.ToIdentifier(namespace, "events")
	if err := cat.CreateNamespace(ctx, catalog.ToIdentifier(namespace), nil); err != nil {
		return nil, nil, errors.Wrap(err, "create namespace")
	}
	schema := iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "name", Type: iceberg.PrimitiveTypes.String, Required: true},
	)
	tbl, err := cat.CreateTable(ctx, identifier, schema)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create unpartitioned table")
	}
	return identifier, tbl, nil
}

type event struct {
	id   int64
	name string
}

func newRecords(schema *iceberg.Schema, rows []event) (array.RecordReader, error) {
	arrowSchema, err := table.SchemaToArrowSchema(schema, nil, true, false)
	if err != nil {
		return nil, errors.Wrap(err, "convert Iceberg schema to Arrow")
	}
	builder := array.NewRecordBuilder(memory.DefaultAllocator, arrowSchema)
	defer builder.Release()
	for _, row := range rows {
		builder.Field(0).(*array.Int64Builder).Append(row.id)
		builder.Field(1).(*array.StringBuilder).Append(row.name)
	}
	batch := builder.NewRecordBatch()
	defer batch.Release()
	reader, err := array.NewRecordReader(arrowSchema, []arrow.RecordBatch{batch})
	if err != nil {
		return nil, errors.Wrap(err, "create Arrow record reader")
	}
	return reader, nil
}

func verifyRows(ctx context.Context, tbl *table.Table, expectedRows int64) error {
	result, err := tbl.Scan().ToArrowTable(ctx)
	if err != nil {
		return errors.Wrap(err, "scan reloaded table")
	}
	defer result.Release()
	if result.NumRows() != expectedRows {
		return fmt.Errorf("scan returned %d rows, want %d", result.NumRows(), expectedRows)
	}
	return nil
}

func inspectObjects(ctx context.Context, client *s3.Client, tbl *table.Table, prefix string) error {
	objects, err := readAllObjects(ctx, client, prefix)
	if err != nil {
		return err
	}
	parsed := make(map[string]bool, len(objects))
	if err := parseMetadataObjects(objects, parsed); err != nil {
		return err
	}
	if err := parseSnapshotObjects(ctx, tbl, objects, parsed); err != nil {
		return err
	}
	if err := ensureAllObjectsParsed(objects, parsed); err != nil {
		return err
	}
	fmt.Printf("Verified all %d Iceberg files in object storage\n", len(objects))
	return nil
}

func parseMetadataObjects(objects map[string][]byte, parsed map[string]bool) error {
	keys := make([]string, 0, len(objects))
	for key := range objects {
		if strings.HasSuffix(key, ".metadata.json") {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		if _, err := table.ParseMetadataBytes(objects[key]); err != nil {
			return errors.Wrapf(err, "parse metadata object %s", key)
		}
		parsed[key] = true
		logParsedObject("table metadata", key, objects[key])
	}
	return nil
}

func parseSnapshotObjects(ctx context.Context, tbl *table.Table, objects map[string][]byte, parsed map[string]bool) error {
	snapshot := tbl.CurrentSnapshot()
	if snapshot == nil {
		return errors.New("reloaded table has no current snapshot")
	}
	manifestListKey, err := objectKey(snapshot.ManifestList)
	if err != nil {
		return err
	}
	manifestList, err := findObject(objects, manifestListKey)
	if err != nil {
		return err
	}
	manifests, err := iceberg.ReadManifestList(bytes.NewReader(manifestList))
	if err != nil {
		return errors.Wrap(err, "parse manifest list")
	}
	parsed[manifestListKey] = true
	logParsedObject("manifest list", manifestListKey, manifestList)
	for _, manifest := range manifests {
		manifestKey, keyErr := objectKey(manifest.FilePath())
		if keyErr != nil {
			return keyErr
		}
		manifestContents, findErr := findObject(objects, manifestKey)
		if findErr != nil {
			return findErr
		}
		entries, readErr := iceberg.ReadManifest(manifest, bytes.NewReader(manifestContents), false)
		if readErr != nil {
			return errors.Wrapf(readErr, "parse manifest %s", manifestKey)
		}
		parsed[manifestKey] = true
		logParsedObject("manifest", manifestKey, manifestContents)
		for _, entry := range entries {
			dataKey, dataKeyErr := objectKey(entry.DataFile().FilePath())
			if dataKeyErr != nil {
				return dataKeyErr
			}
			dataContents, findErr := findObject(objects, dataKey)
			if findErr != nil {
				return findErr
			}
			if parseErr := parseParquet(ctx, dataContents); parseErr != nil {
				return errors.Wrapf(parseErr, "parse data file %s", dataKey)
			}
			parsed[dataKey] = true
			logParsedObject("Parquet data", dataKey, dataContents)
		}
	}
	return nil
}

func logParsedObject(kind, key string, contents []byte) {
	fmt.Printf("  - %-14s %s (%d bytes)\n", kind+":", key, len(contents))
}

func findObject(objects map[string][]byte, key string) ([]byte, error) {
	contents, ok := objects[key]
	if !ok {
		return nil, fmt.Errorf("referenced Iceberg object %s was not found", key)
	}
	return contents, nil
}

func ensureAllObjectsParsed(objects map[string][]byte, parsed map[string]bool) error {
	for key := range objects {
		if !parsed[key] {
			return fmt.Errorf("unrecognized Iceberg object %s", key)
		}
	}
	return nil
}

func readAllObjects(ctx context.Context, client *s3.Client, prefix string) (map[string][]byte, error) {
	objects := make(map[string][]byte)
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "list RustFS objects")
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			output, getErr := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: object.Key})
			if getErr != nil {
				return nil, errors.Wrapf(getErr, "get RustFS object %s", key)
			}
			contents, readErr := io.ReadAll(output.Body)
			closeErr := output.Body.Close()
			if readErr != nil {
				return nil, errors.Wrapf(readErr, "read RustFS object %s", key)
			}
			if closeErr != nil {
				return nil, errors.Wrapf(closeErr, "close RustFS object %s", key)
			}
			objects[key] = contents
		}
	}
	return objects, nil
}

func objectKey(location string) (string, error) {
	prefix := "s3://" + bucket + "/"
	if !strings.HasPrefix(location, prefix) {
		return "", fmt.Errorf("object location %q is outside %s", location, prefix)
	}
	return strings.TrimPrefix(location, prefix), nil
}

func parseParquet(ctx context.Context, contents []byte) error {
	reader, err := file.NewParquetReader(bytes.NewReader(contents))
	if err != nil {
		return errors.Wrap(err, "open Parquet reader")
	}
	defer reader.Close()
	arrowReader, err := pqarrow.NewFileReader(reader, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
	if err != nil {
		return errors.Wrap(err, "open Parquet Arrow reader")
	}
	result, err := arrowReader.ReadTable(ctx)
	if err != nil {
		return errors.Wrap(err, "read Parquet table")
	}
	result.Release()
	return nil
}

// IOTrace records logical AWS operations in call order.
type IOTrace struct {
	mu         sync.Mutex
	operations []string
}

// NewIOTrace returns an empty logical I/O trace.
func NewIOTrace() *IOTrace {
	return &IOTrace{}
}

// BeforeIO records an operation before the AWS SDK retry loop.
func (t *IOTrace) BeforeIO(_ context.Context, operation string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.operations = append(t.operations, operation)
	return nil
}

// Operations returns the recorded operations in call order.
func (t *IOTrace) Operations() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.operations)
}
