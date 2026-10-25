//go:build specguardian

// Shared infrastructure for the generated conformance tests.
package specguardian_test

import (
	"database/sql"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-sql-driver/mysql"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gawssdkgov2"
	"github.com/mrasu/spec-guardian/guardian/gsql"
	"github.com/stretchr/testify/require"
)

const (
	databaseDSN    = "root@tcp(localhost:13306)/idempotent_request?parseTime=true"
	dynamoEndpoint = "http://localhost:14566"
	dynamoRegion   = "us-east-1"
)

type testResources struct {
	actionDB     *sql.DB
	actionDynamo *dynamodb.Client
}

func newTestResources(t *testing.T, controller *guardian.InjectionController) *testResources {
	t.Helper()

	actionDB := openDatabase(t, controller)
	t.Cleanup(func() {
		require.NoError(t, actionDB.Close())
	})
	actionDynamo := newDynamoClient(controller)
	resources := &testResources{actionDB: actionDB, actionDynamo: actionDynamo}
	return resources
}

func openDatabase(t *testing.T, fault guardian.Fault) *sql.DB {
	t.Helper()
	configuration, err := mysql.ParseDSN(databaseDSN)
	require.NoError(t, err)
	connector, err := mysql.NewConnector(configuration)
	require.NoError(t, err)
	connector = gsql.NewConnector(connector, fault)
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)
	// A fault before rollback can leave a transaction open. Close returned
	// connections so provider observation and cleanup cannot inherit its locks.
	database.SetMaxIdleConns(0)
	require.NoError(t, database.PingContext(t.Context()))
	return database
}

func newDynamoClient(fault guardian.Fault) *dynamodb.Client {
	options := dynamodb.Options{
		BaseEndpoint: aws.String(dynamoEndpoint), Region: dynamoRegion,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("test", "test", "")), Retryer: aws.NopRetryer{},
	}
	interceptors := smithyhttp.InterceptorRegistry{}
	interceptors.AddBeforeExecution(gawssdkgov2.NewInterceptor(fault))
	options.Interceptors = interceptors
	return dynamodb.New(options)
}
