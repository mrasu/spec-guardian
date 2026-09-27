package gsql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"

	"github.com/mrasu/spec-guardian/guardian"
)

var (
	_ driver.Connector          = (*connectorHook)(nil)
	_ io.Closer                 = (*connectorHook)(nil)
	_ driver.Conn               = (*connHook)(nil)
	_ driver.ConnBeginTx        = (*connHook)(nil)
	_ driver.ConnPrepareContext = (*connHook)(nil)
	_ driver.ExecerContext      = (*connHook)(nil)
	_ driver.QueryerContext     = (*connHook)(nil)
	_ driver.Pinger             = (*connHook)(nil)
	_ driver.SessionResetter    = (*connHook)(nil)
	_ driver.Validator          = (*connHook)(nil)
	_ driver.NamedValueChecker  = (*connHook)(nil)
	_ driver.Tx                 = (*txHook)(nil)
	_ driver.Stmt               = (*stmtHook)(nil)
	_ driver.StmtExecContext    = (*stmtHook)(nil)
	_ driver.StmtQueryContext   = (*stmtHook)(nil)
	_ driver.NamedValueChecker  = (*stmtHook)(nil)
	_ driver.ColumnConverter    = (*stmtHook)(nil)
)

// NewConnector wraps connector with SQL fault-injection points controlled by fault.
func NewConnector(connector driver.Connector, fault guardian.Fault) driver.Connector {
	return &connectorHook{Connector: connector, fault: fault}
}

type connectorHook struct {
	driver.Connector
	fault guardian.Fault
}

func (c *connectorHook) Connect(ctx context.Context) (driver.Conn, error) {
	connection, err := c.Connector.Connect(ctx)
	if err != nil {
		return connection, err
	}
	// Fault-targeted operations must use their context-aware interfaces. Validate
	// them once here so an unsupported driver cannot bypass an injection point via
	// a database/sql fallback, and store the validated interfaces in connHook.
	beginner, ok := connection.(driver.ConnBeginTx)
	if !ok {
		return nil, closeUnsupportedConnection(connection, "driver.ConnBeginTx")
	}
	execer, ok := connection.(driver.ExecerContext)
	if !ok {
		return nil, closeUnsupportedConnection(connection, "driver.ExecerContext")
	}
	queryer, ok := connection.(driver.QueryerContext)
	if !ok {
		return nil, closeUnsupportedConnection(connection, "driver.QueryerContext")
	}
	preparer, ok := connection.(driver.ConnPrepareContext)
	if !ok {
		return nil, closeUnsupportedConnection(connection, "driver.ConnPrepareContext")
	}
	return &connHook{
		Conn:     connection,
		beginner: beginner,
		execer:   execer,
		queryer:  queryer,
		preparer: preparer,
		fault:    c.fault,
	}, nil
}

func closeUnsupportedConnection(connection driver.Conn, interfaceName string) error {
	unsupported := fmt.Errorf("spec-guardian: driver connection does not implement %s", interfaceName)
	if err := connection.Close(); err != nil {
		return err
	}
	return unsupported
}

func (c *connectorHook) Close() error {
	closer, ok := c.Connector.(io.Closer)
	if !ok {
		return nil
	}
	return closer.Close()
}

// connHook stores the fault-targeted interfaces validated by Connect. Other
// optional connection interfaces are checked when called because they are not
// injection points and their documented absence behavior can be preserved.
type connHook struct {
	driver.Conn
	beginner driver.ConnBeginTx
	execer   driver.ExecerContext
	queryer  driver.QueryerContext
	preparer driver.ConnPrepareContext
	fault    guardian.Fault
}

func (c *connHook) Begin() (driver.Tx, error) {
	if err := c.fault.BeforeIO(context.Background(), "sql.BeginTx"); err != nil {
		return nil, err
	}
	transaction, err := c.Conn.Begin()
	if err != nil {
		return transaction, err
	}
	return &txHook{Tx: transaction, fault: c.fault}, nil
}

func (c *connHook) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *connHook) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := c.fault.BeforeIO(ctx, "sql.PrepareContext"); err != nil {
		return nil, err
	}
	statement, err := c.preparer.PrepareContext(ctx, query)
	if err != nil {
		return statement, err
	}
	// Statement interfaces cannot be validated until PrepareContext returns the
	// driver statement. Both are required so database/sql cannot fall back to an
	// unhooked statement operation.
	execer, ok := statement.(driver.StmtExecContext)
	if !ok {
		return nil, closeUnsupportedStatement(statement, "driver.StmtExecContext")
	}
	queryer, ok := statement.(driver.StmtQueryContext)
	if !ok {
		return nil, closeUnsupportedStatement(statement, "driver.StmtQueryContext")
	}
	return &stmtHook{Stmt: statement, execer: execer, queryer: queryer, fault: c.fault}, nil
}

func closeUnsupportedStatement(statement driver.Stmt, interfaceName string) error {
	unsupported := fmt.Errorf("spec-guardian: driver statement does not implement %s", interfaceName)
	if err := statement.Close(); err != nil {
		return err
	}
	return unsupported
}

func (c *connHook) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if err := c.fault.BeforeIO(ctx, "sql.BeginTx"); err != nil {
		return nil, err
	}
	transaction, err := c.beginner.BeginTx(ctx, options)
	if err != nil {
		return transaction, err
	}
	return &txHook{Tx: transaction, fault: c.fault}, nil
}

func (c *connHook) ExecContext(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Result, error) {
	if err := c.fault.BeforeIO(ctx, "sql.ExecContext"); err != nil {
		return nil, err
	}
	return c.execer.ExecContext(ctx, query, arguments)
}

func (c *connHook) QueryContext(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Rows, error) {
	if err := c.fault.BeforeIO(ctx, "sql.QueryContext"); err != nil {
		return nil, err
	}
	return c.queryer.QueryContext(ctx, query, arguments)
}

// The following methods preserve non-targeted optional driver behavior. When
// the underlying interface is absent, each method returns the same result that
// database/sql uses for an implementation that does not advertise it.
func (c *connHook) Ping(ctx context.Context) error {
	pinger, ok := c.Conn.(driver.Pinger)
	if !ok {
		return nil
	}
	return pinger.Ping(ctx)
}

func (c *connHook) ResetSession(ctx context.Context) error {
	resetter, ok := c.Conn.(driver.SessionResetter)
	if !ok {
		return nil
	}
	return resetter.ResetSession(ctx)
}

func (c *connHook) IsValid() bool {
	validator, ok := c.Conn.(driver.Validator)
	if !ok {
		return true
	}
	return validator.IsValid()
}

func (c *connHook) CheckNamedValue(value *driver.NamedValue) error {
	checker, ok := c.Conn.(driver.NamedValueChecker)
	if !ok {
		return driver.ErrSkip
	}
	return checker.CheckNamedValue(value)
}

type txHook struct {
	driver.Tx
	fault guardian.Fault
}

// stmtHook stores the fault-targeted interfaces validated by PrepareContext.
// Non-targeted conversion interfaces are checked when called so their absence
// retains database/sql's standard conversion behavior.
type stmtHook struct {
	driver.Stmt
	execer  driver.StmtExecContext
	queryer driver.StmtQueryContext
	fault   guardian.Fault
}

func (s *stmtHook) Exec(arguments []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), namedValues(arguments))
}

func (s *stmtHook) ExecContext(ctx context.Context, arguments []driver.NamedValue) (driver.Result, error) {
	if err := s.fault.BeforeIO(ctx, "sql.StmtExecContext"); err != nil {
		return nil, err
	}
	return s.execer.ExecContext(ctx, arguments)
}

func (s *stmtHook) Query(arguments []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), namedValues(arguments))
}

func (s *stmtHook) QueryContext(ctx context.Context, arguments []driver.NamedValue) (driver.Rows, error) {
	if err := s.fault.BeforeIO(ctx, "sql.StmtQueryContext"); err != nil {
		return nil, err
	}
	return s.queryer.QueryContext(ctx, arguments)
}

func (s *stmtHook) CheckNamedValue(value *driver.NamedValue) error {
	checker, ok := s.Stmt.(driver.NamedValueChecker)
	if !ok {
		return driver.ErrSkip
	}
	return checker.CheckNamedValue(value)
}

func (s *stmtHook) ColumnConverter(index int) driver.ValueConverter {
	converter, ok := s.Stmt.(driver.ColumnConverter)
	if !ok {
		return driver.DefaultParameterConverter
	}
	return converter.ColumnConverter(index)
}

func namedValues(arguments []driver.Value) []driver.NamedValue {
	named := make([]driver.NamedValue, len(arguments))
	for index, value := range arguments {
		named[index] = driver.NamedValue{Ordinal: index + 1, Value: value}
	}
	return named
}

func (t *txHook) Commit() error {
	if err := t.fault.BeforeIO(context.Background(), "sql.Commit"); err != nil {
		// database/sql marks a transaction done when its driver's Commit returns an
		// error. Roll back the underlying transaction here.
		_ = t.Tx.Rollback()
		return err
	}
	return t.Tx.Commit()
}

func (t *txHook) Rollback() error {
	if err := t.fault.BeforeIO(context.Background(), "sql.Rollback"); err != nil {
		return err
	}
	return t.Tx.Rollback()
}
