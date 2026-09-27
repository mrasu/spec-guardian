package gsql

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pgxTestDatabaseURL = "postgres://postgres@localhost:15432/postgres?sslmode=disable"

var (
	_ driver.ConnBeginTx        = (*stdlib.Conn)(nil)
	_ driver.ConnPrepareContext = (*stdlib.Conn)(nil)
	_ driver.ExecerContext      = (*stdlib.Conn)(nil)
	_ driver.QueryerContext     = (*stdlib.Conn)(nil)
	_ driver.StmtExecContext    = (*stdlib.Stmt)(nil)
	_ driver.StmtQueryContext   = (*stdlib.Stmt)(nil)
)

func TestPGXConnectorIsSupported(t *testing.T) {
	config, err := pgx.ParseConfig(pgxTestDatabaseURL)
	require.NoError(t, err)
	connector := stdlib.GetConnector(*config)
	require.NotNil(t, connector)
	require.NotNil(t, connector.Driver())
}

func TestPGXIntegration(t *testing.T) {
	t.Run("success path", func(t *testing.T) {
		fault := &recordingFault{}
		database := openPGXDatabase(t, fault)

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

		assert.Contains(t, fault.operations, "sql.BeginTx")
		assert.Contains(t, fault.operations, "sql.ExecContext")
		assert.Contains(t, fault.operations, "sql.QueryContext")
		assert.Contains(t, fault.operations, "sql.Commit")
		assert.Contains(t, fault.operations, "sql.Rollback")
		assert.Contains(t, fault.operations, "sql.PrepareContext")
		assert.Contains(t, fault.operations, "sql.StmtExecContext")
		assert.Contains(t, fault.operations, "sql.StmtQueryContext")
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
			database := openPGXDatabase(t, fault)
			err := operation.invoke(t, database)
			assert.ErrorIs(t, err, errInjected)
		})
	}
}

func openPGXDatabase(t *testing.T, fault *recordingFault) *sql.DB {
	t.Helper()
	config, err := pgx.ParseConfig(pgxTestDatabaseURL)
	require.NoError(t, err)
	database := sql.OpenDB(NewConnector(stdlib.GetConnector(*config), fault))
	database.SetMaxOpenConns(1)
	require.NoError(t, database.PingContext(t.Context()))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	return database
}

func invokePrepared(invoke func(*testing.T, *sql.Stmt) error) func(*testing.T, *sql.DB) error {
	return func(t *testing.T, database *sql.DB) error {
		statement, err := database.PrepareContext(t.Context(), "SELECT 1")
		if err != nil {
			return err
		}
		defer statement.Close()
		return invoke(t, statement)
	}
}
