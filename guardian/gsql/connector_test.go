package gsql

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errInjected = errors.New("injected")

type recordingFault struct {
	operations []string
	injectAt   string
}

func (f *recordingFault) BeforeIO(_ context.Context, operation string) error {
	f.operations = append(f.operations, operation)
	if operation == f.injectAt {
		return errInjected
	}
	return nil
}

type stubConnector struct {
	connection driver.Conn
	err        error
}

func (c stubConnector) Connect(context.Context) (driver.Conn, error) { return c.connection, c.err }
func (stubConnector) Driver() driver.Driver                          { return stubDriver{} }

type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) { return nil, errors.New("not implemented") }

type stubConn struct {
	began       bool
	executed    bool
	queried     bool
	prepared    bool
	transaction *stubTx
	statement   driver.Stmt
}

func (c *stubConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c *stubConn) Close() error                        { return nil }
func (c *stubConn) Begin() (driver.Tx, error) {
	c.began = true
	return c.transaction, nil
}
func (c *stubConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.began = true
	return c.transaction, nil
}
func (c *stubConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.executed = true
	return driver.RowsAffected(1), nil
}
func (c *stubConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.queried = true
	return stubRows{}, nil
}
func (c *stubConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	c.prepared = true
	if c.statement == nil {
		return &stubStmt{}, nil
	}
	return c.statement, nil
}

type stubStmt struct {
	executed bool
	queried  bool
	closed   bool
}

func (s *stubStmt) Close() error { s.closed = true; return nil }
func (*stubStmt) NumInput() int  { return -1 }
func (s *stubStmt) Exec([]driver.Value) (driver.Result, error) {
	s.executed = true
	return driver.RowsAffected(1), nil
}
func (s *stubStmt) Query([]driver.Value) (driver.Rows, error) {
	s.queried = true
	return stubRows{}, nil
}
func (s *stubStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	s.executed = true
	return driver.RowsAffected(1), nil
}
func (s *stubStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	s.queried = true
	return stubRows{}, nil
}

type stubTx struct {
	committed  bool
	rolledBack bool
}

func (t *stubTx) Commit() error {
	t.committed = true
	return nil
}
func (t *stubTx) Rollback() error {
	t.rolledBack = true
	return nil
}

type stubRows struct{}

func (stubRows) Columns() []string         { return nil }
func (stubRows) Close() error              { return nil }
func (stubRows) Next([]driver.Value) error { return io.EOF }

func TestConnectorHooksSQLOperations(t *testing.T) {
	fault := &recordingFault{}
	baseTransaction := &stubTx{}
	baseConnection := &stubConn{transaction: baseTransaction}
	connector := NewConnector(stubConnector{connection: baseConnection}, fault)
	connection, err := connector.Connect(t.Context())
	require.NoError(t, err)

	contextConnection := connection.(interface {
		BeginTx(context.Context, driver.TxOptions) (driver.Tx, error)
		ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error)
		QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error)
	})
	transaction, err := contextConnection.BeginTx(t.Context(), driver.TxOptions{})
	require.NoError(t, err)
	_, err = contextConnection.ExecContext(t.Context(), "ignored", nil)
	require.NoError(t, err)
	_, err = contextConnection.QueryContext(t.Context(), "ignored", nil)
	require.NoError(t, err)
	require.NoError(t, transaction.Commit())

	transaction, err = contextConnection.BeginTx(t.Context(), driver.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, transaction.Rollback())
	assert.Equal(t, []string{
		"sql.BeginTx", "sql.ExecContext", "sql.QueryContext", "sql.Commit", "sql.BeginTx", "sql.Rollback",
	}, fault.operations)
	assert.True(t, baseConnection.began)
	assert.True(t, baseConnection.executed)
	assert.True(t, baseConnection.queried)
	assert.True(t, baseTransaction.committed)
	assert.True(t, baseTransaction.rolledBack)
}

func TestConnectorDoesNotCallDriverWhenFaultIsInjected(t *testing.T) {
	fault := &recordingFault{injectAt: "sql.ExecContext"}
	baseConnection := &stubConn{transaction: &stubTx{}}
	connector := NewConnector(stubConnector{connection: baseConnection}, fault)
	connection, err := connector.Connect(t.Context())
	require.NoError(t, err)

	_, err = connection.(driver.ExecerContext).ExecContext(t.Context(), "ignored", nil)
	assert.ErrorIs(t, err, errInjected)
	assert.False(t, baseConnection.executed)
}

func TestConnectorRollsBackDriverTransactionWhenCommitFaultIsInjected(t *testing.T) {
	fault := &recordingFault{injectAt: "sql.Commit"}
	baseTransaction := &stubTx{}
	baseConnection := &stubConn{transaction: baseTransaction}
	connection := connectForTest(t, baseConnection, fault)

	transaction, err := connection.(driver.ConnBeginTx).BeginTx(t.Context(), driver.TxOptions{})
	require.NoError(t, err)
	err = transaction.Commit()

	assert.ErrorIs(t, err, errInjected)
	assert.False(t, baseTransaction.committed)
	assert.True(t, baseTransaction.rolledBack)
}

func TestConnectorRejectsMissingTargetInterfaces(t *testing.T) {
	tests := []struct {
		name       string
		connection driver.Conn
		wantError  string
	}{
		{
			name:       "ConnBeginTx",
			connection: minimalConn{},
			wantError:  "spec-guardian: driver connection does not implement driver.ConnBeginTx",
		},
		{
			name:       "ExecerContext",
			connection: beginOnlyConn{},
			wantError:  "spec-guardian: driver connection does not implement driver.ExecerContext",
		},
		{
			name:       "QueryerContext",
			connection: beginExecConn{},
			wantError:  "spec-guardian: driver connection does not implement driver.QueryerContext",
		},
		{
			name:       "ConnPrepareContext",
			connection: beginExecQueryConn{},
			wantError:  "spec-guardian: driver connection does not implement driver.ConnPrepareContext",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connector := NewConnector(stubConnector{connection: test.connection}, &recordingFault{})
			connection, err := connector.Connect(t.Context())
			assert.Nil(t, connection)
			assert.EqualError(t, err, test.wantError)
		})
	}
}

func TestConnectorClosesUnsupportedConnection(t *testing.T) {
	closeError := errors.New("close failed")
	connection := &unsupportedConn{closeError: closeError}
	connector := NewConnector(stubConnector{connection: connection}, &recordingFault{})

	hooked, err := connector.Connect(t.Context())
	assert.Nil(t, hooked)
	assert.Same(t, closeError, err)
	assert.True(t, connection.closed)
}

func TestConnectorPreservesUnderlyingConnectResultAndError(t *testing.T) {
	connection := &minimalConn{}
	connectError := errors.New("connect failed")
	connector := NewConnector(stubConnector{connection: connection, err: connectError}, &recordingFault{})

	got, err := connector.Connect(t.Context())
	assert.Same(t, connection, got)
	assert.Same(t, connectError, err)
}

type unsupportedConn struct {
	minimalConn
	closed     bool
	closeError error
}

func (c *unsupportedConn) Close() error {
	c.closed = true
	return c.closeError
}

type minimalConn struct{}

func (minimalConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (minimalConn) Close() error                        { return nil }
func (minimalConn) Begin() (driver.Tx, error)           { return &stubTx{}, nil }

type beginOnlyConn struct{ minimalConn }

func (beginOnlyConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &stubTx{}, nil
}

type beginExecConn struct{ beginOnlyConn }

func (beginExecConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

type beginExecQueryConn struct{ beginExecConn }

func (beginExecQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return stubRows{}, nil
}

func TestConnectorForwardsConnectionLifecycleInterfaces(t *testing.T) {
	base := &lifecycleConn{}
	hooked := connectForTest(t, base, &recordingFault{})

	require.NoError(t, hooked.(driver.Pinger).Ping(t.Context()))
	require.NoError(t, hooked.(driver.SessionResetter).ResetSession(t.Context()))
	assert.False(t, hooked.(driver.Validator).IsValid())
	value := &driver.NamedValue{Value: "before"}
	require.NoError(t, hooked.(driver.NamedValueChecker).CheckNamedValue(value))
	assert.Equal(t, "after", value.Value)
	assert.True(t, base.pinged)
	assert.True(t, base.reset)
}

type lifecycleConn struct {
	stubConn
	pinged bool
	reset  bool
}

func (c *lifecycleConn) Ping(context.Context) error {
	c.pinged = true
	return nil
}
func (c *lifecycleConn) ResetSession(context.Context) error {
	c.reset = true
	return nil
}
func (*lifecycleConn) IsValid() bool { return false }
func (*lifecycleConn) CheckNamedValue(value *driver.NamedValue) error {
	value.Value = "after"
	return nil
}

func TestConnectorPreservesUnderlyingResultsAndErrors(t *testing.T) {
	hooked := connectForTest(t, failingConn{}, &recordingFault{})

	transaction, err := hooked.Begin()
	assert.Same(t, failedTransaction, transaction)
	assert.Same(t, errBeginFailed, err)
	transaction, err = hooked.(driver.ConnBeginTx).BeginTx(t.Context(), driver.TxOptions{})
	assert.Same(t, failedTransaction, transaction)
	assert.Same(t, errBeginTxFailed, err)
	statement, err := hooked.(driver.ConnPrepareContext).PrepareContext(t.Context(), "ignored")
	assert.Same(t, failedStatement, statement)
	assert.Same(t, errPrepareFailed, err)
	result, err := hooked.(driver.ExecerContext).ExecContext(t.Context(), "ignored", nil)
	assert.Equal(t, driver.RowsAffected(1), result)
	assert.Same(t, errExecFailed, err)
	rows, err := hooked.(driver.QueryerContext).QueryContext(t.Context(), "ignored", nil)
	assert.Equal(t, stubRows{}, rows)
	assert.Same(t, errQueryFailed, err)
}

func TestConnectorPreservesDriverErrSkip(t *testing.T) {
	hooked := connectForTest(t, skippingConn{}, &recordingFault{})

	result, err := hooked.(driver.ExecerContext).ExecContext(t.Context(), "ignored", nil)
	assert.Nil(t, result)
	assert.Same(t, errExecSkip, err)
	rows, err := hooked.(driver.QueryerContext).QueryContext(t.Context(), "ignored", nil)
	assert.Nil(t, rows)
	assert.Same(t, driver.ErrSkip, err)
	transaction, err := hooked.(driver.ConnBeginTx).BeginTx(t.Context(), driver.TxOptions{})
	assert.Nil(t, transaction)
	assert.Same(t, driver.ErrSkip, err)
	statement, err := hooked.(driver.ConnPrepareContext).PrepareContext(t.Context(), "ignored")
	assert.Nil(t, statement)
	assert.Same(t, errPrepareSkip, err)
}

type skippingConn struct{ minimalConn }

var (
	errExecSkip    = fmt.Errorf("wrapped: %w", driver.ErrSkip)
	errPrepareSkip = fmt.Errorf("wrapped: %w", driver.ErrSkip)
)

func (skippingConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, errExecSkip
}
func (skippingConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, driver.ErrSkip
}
func (skippingConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return nil, driver.ErrSkip
}
func (skippingConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	return nil, errPrepareSkip
}

type failingConn struct{ minimalConn }

var (
	failedTransaction = &stubTx{}
	failedStatement   = &stubStmt{}
	errBeginFailed    = errors.New("begin failed")
	errBeginTxFailed  = errors.New("begin tx failed")
	errPrepareFailed  = errors.New("prepare failed")
	errExecFailed     = errors.New("exec failed")
	errQueryFailed    = errors.New("query failed")
)

func (failingConn) Begin() (driver.Tx, error) {
	return failedTransaction, errBeginFailed
}
func (failingConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return failedTransaction, errBeginTxFailed
}

func (failingConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), errExecFailed
}
func (failingConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return stubRows{}, errQueryFailed
}
func (failingConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	return failedStatement, errPrepareFailed
}

func TestConnectorHooksPreparedStatements(t *testing.T) {
	fault := &recordingFault{}
	baseStatement := &stubStmt{}
	baseConnection := &stubConn{transaction: &stubTx{}, statement: baseStatement}
	hooked := connectForTest(t, baseConnection, fault)

	statement, err := hooked.(driver.ConnPrepareContext).PrepareContext(t.Context(), "ignored")
	require.NoError(t, err)
	_, err = statement.(driver.StmtExecContext).ExecContext(t.Context(), nil)
	require.NoError(t, err)
	_, err = statement.(driver.StmtQueryContext).QueryContext(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"sql.PrepareContext", "sql.StmtExecContext", "sql.StmtQueryContext"}, fault.operations)
	assert.True(t, baseConnection.prepared)
	assert.True(t, baseStatement.executed)
	assert.True(t, baseStatement.queried)
}

func TestConnectorPreservesStatementDriverErrSkip(t *testing.T) {
	baseConnection := &stubConn{transaction: &stubTx{}, statement: &skippingStmt{}}
	hooked := connectForTest(t, baseConnection, &recordingFault{})
	statement, err := hooked.(driver.ConnPrepareContext).PrepareContext(t.Context(), "ignored")
	require.NoError(t, err)

	result, err := statement.(driver.StmtExecContext).ExecContext(t.Context(), nil)
	assert.Nil(t, result)
	assert.Same(t, errStmtExecSkip, err)
	rows, err := statement.(driver.StmtQueryContext).QueryContext(t.Context(), nil)
	assert.Nil(t, rows)
	assert.Same(t, driver.ErrSkip, err)
}

type skippingStmt struct{ stubStmt }

var errStmtExecSkip = fmt.Errorf("wrapped: %w", driver.ErrSkip)

func (skippingStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	return nil, errStmtExecSkip
}
func (skippingStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	return nil, driver.ErrSkip
}

func TestConnectorRejectsMissingStatementInterfaces(t *testing.T) {
	tests := []struct {
		name      string
		statement driver.Stmt
		wantError string
	}{
		{
			name:      "StmtExecContext",
			statement: &minimalStmt{},
			wantError: "spec-guardian: driver statement does not implement driver.StmtExecContext",
		},
		{
			name:      "StmtQueryContext",
			statement: &execOnlyStmt{},
			wantError: "spec-guardian: driver statement does not implement driver.StmtQueryContext",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseConnection := &stubConn{transaction: &stubTx{}, statement: test.statement}
			hooked := connectForTest(t, baseConnection, &recordingFault{})
			statement, err := hooked.(driver.ConnPrepareContext).PrepareContext(t.Context(), "ignored")
			assert.Nil(t, statement)
			assert.EqualError(t, err, test.wantError)
		})
	}
}

type minimalStmt struct{ closed bool }

func (s *minimalStmt) Close() error { s.closed = true; return nil }
func (*minimalStmt) NumInput() int  { return -1 }
func (*minimalStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (*minimalStmt) Query([]driver.Value) (driver.Rows, error) { return stubRows{}, nil }

type execOnlyStmt struct{ minimalStmt }

func (*execOnlyStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func connectForTest(t *testing.T, connection driver.Conn, fault *recordingFault) driver.Conn {
	t.Helper()
	connector := NewConnector(stubConnector{connection: connection}, fault)
	hooked, err := connector.Connect(t.Context())
	require.NoError(t, err)
	return hooked
}

func TestConnectorForwardsClose(t *testing.T) {
	base := &closableConnector{stubConnector: stubConnector{connection: &stubConn{transaction: &stubTx{}}}}
	hooked := NewConnector(base, &recordingFault{})

	require.NoError(t, hooked.(io.Closer).Close())
	assert.True(t, base.closed)
}

type closableConnector struct {
	stubConnector
	closed bool
}

func (c *closableConnector) Close() error {
	c.closed = true
	return nil
}
