package gsql

import (
	"database/sql"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mysqlTestDatabaseURL = "root@tcp(localhost:13306)/mysql"

func TestMySQLConnectorIsSupported(t *testing.T) {
	config, err := mysql.ParseDSN(mysqlTestDatabaseURL)
	require.NoError(t, err)
	connector, err := mysql.NewConnector(config)
	require.NoError(t, err)
	require.NotNil(t, connector)
	require.NotNil(t, connector.Driver())
}

func TestMySQLIntegration(t *testing.T) {
	t.Run("success path", func(t *testing.T) {
		fault := &recordingFault{}
		database := openMySQLDatabase(t, fault)

		transaction, err := database.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		_, err = transaction.ExecContext(t.Context(), "SELECT 1")
		require.NoError(t, err)
		require.NoError(t, transaction.Commit())

		transaction, err = database.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		require.NoError(t, transaction.Rollback())

		rows, err := database.QueryContext(t.Context(), "SELECT 1")
		require.NoError(t, err)
		require.NoError(t, rows.Close())

		statement, err := database.PrepareContext(t.Context(), "SELECT 1")
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, statement.Close()) })
		_, err = statement.ExecContext(t.Context())
		require.NoError(t, err)
		rows, err = statement.QueryContext(t.Context())
		require.NoError(t, err)
		require.NoError(t, rows.Close())

		assert.Equal(t, []string{
			"sql.BeginTx", "sql.ExecContext", "sql.Commit", "sql.BeginTx", "sql.Rollback",
			"sql.QueryContext", "sql.PrepareContext", "sql.StmtExecContext", "sql.StmtQueryContext",
		}, fault.operations)
	})

	operations := []struct {
		name   string
		invoke func(*testing.T, *sql.DB) error
	}{
		{name: "sql.BeginTx", invoke: func(t *testing.T, database *sql.DB) error {
			_, err := database.BeginTx(t.Context(), nil)
			return err
		}},
		{name: "sql.ExecContext", invoke: func(t *testing.T, database *sql.DB) error {
			_, err := database.ExecContext(t.Context(), "SELECT 1")
			return err
		}},
		{name: "sql.QueryContext", invoke: func(t *testing.T, database *sql.DB) error {
			_, err := database.QueryContext(t.Context(), "SELECT 1")
			return err
		}},
		{name: "sql.Commit", invoke: func(t *testing.T, database *sql.DB) error {
			transaction, err := database.BeginTx(t.Context(), nil)
			if err != nil {
				return err
			}
			defer transaction.Rollback()
			return transaction.Commit()
		}},
		{name: "sql.Rollback", invoke: func(t *testing.T, database *sql.DB) error {
			transaction, err := database.BeginTx(t.Context(), nil)
			if err != nil {
				return err
			}
			return transaction.Rollback()
		}},
		{name: "sql.PrepareContext", invoke: func(t *testing.T, database *sql.DB) error {
			_, err := database.PrepareContext(t.Context(), "SELECT 1")
			return err
		}},
		{name: "sql.StmtExecContext", invoke: invokePrepared(func(t *testing.T, statement *sql.Stmt) error {
			_, err := statement.ExecContext(t.Context())
			return err
		})},
		{name: "sql.StmtQueryContext", invoke: invokePrepared(func(t *testing.T, statement *sql.Stmt) error {
			_, err := statement.QueryContext(t.Context())
			return err
		})},
	}

	for _, operation := range operations {
		t.Run(operation.name+" fault", func(t *testing.T) {
			fault := &recordingFault{injectAt: operation.name}
			database := openMySQLDatabase(t, fault)
			err := operation.invoke(t, database)
			assert.ErrorIs(t, err, errInjected)
		})
	}
}

func openMySQLDatabase(t *testing.T, fault *recordingFault) *sql.DB {
	t.Helper()
	config, err := mysql.ParseDSN(mysqlTestDatabaseURL)
	require.NoError(t, err)
	connector, err := mysql.NewConnector(config)
	require.NoError(t, err)
	database := sql.OpenDB(NewConnector(connector, fault))
	database.SetMaxOpenConns(1)
	require.NoError(t, database.PingContext(t.Context()))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	return database
}
