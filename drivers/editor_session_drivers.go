package drivers

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
)

var (
	_ EditorSessionDriver = (*Postgres)(nil)
	_ EditorSessionDriver = (*MySQL)(nil)
	_ EditorSessionDriver = (*MSSQL)(nil)
	_ EditorSessionDriver = (*SQLite)(nil)
	_ EditorSessionDriver = (*ClickHouse)(nil)
)

// reserveEditorConn reserves a pooled connection and, for a non-login
// database, switches it with use. The session discards the connection rather
// than pooling it, so the switched database never leaks.
func reserveEditorConn(ctx context.Context, pool *sql.DB, use string) (*sql.Conn, func(), error) {
	ctx = contextOrBackground(ctx)
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	if use != "" {
		if _, err := conn.ExecContext(ctx, use); err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
	}
	return conn, nil, nil
}

// --- MySQL / MariaDB ---------------------------------------------------------

func (db *MySQL) editorSession() *editorSession {
	return editorSessionFor(db, func() *editorSession {
		return newEditorSession(func(ctx context.Context, database string) (*sql.Conn, func(), error) {
			use := ""
			if database != db.CurrentDatabase {
				use = "USE `" + strings.ReplaceAll(database, "`", "``") + "`"
			}
			return reserveEditorConn(ctx, db.Connection, use)
		}, mysqlTransactionOpen)
	})
}

func (db *MySQL) editorDatabase(database string) string {
	if database == "" {
		return db.CurrentDatabase
	}
	return database
}

func (db *MySQL) EditorStreamQuery(ctx context.Context, database, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error) {
	return db.editorSession().streamQuery(ctx, db.editorDatabase(database), query, maxRows, onBatch)
}

func (db *MySQL) EditorExec(ctx context.Context, database, query string) (string, error) {
	return db.editorSession().exec(ctx, db.editorDatabase(database), query)
}

func (db *MySQL) EditorTransactionOpen() bool { return transactionOpenFor(db) }

func (db *MySQL) CloseEditorSession() error { return closeEditorSessionFor(db) }

// mysqlStatusInTrans is SERVER_STATUS_IN_TRANS from the MySQL protocol.
const mysqlStatusInTrans = 1

// mysqlTransactionOpen reads the SERVER_STATUS_IN_TRANS flag the server sent
// with its last reply. MySQL has no SQL-level equivalent (only MariaDB has
// @@in_transaction), and go-sql-driver/mysql keeps the flag unexported, so it
// is read by reflection; if the driver's layout changes, the session still
// works but cannot report the state.
func mysqlTransactionOpen(_ context.Context, conn *sql.Conn) (bool, error) {
	open := false
	err := conn.Raw(func(driverConn any) error {
		v := reflect.ValueOf(driverConn)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil
		}
		status := v.FieldByName("status")
		if !status.IsValid() || !status.CanUint() {
			return nil
		}
		open = status.Uint()&mysqlStatusInTrans != 0
		return nil
	})
	return open, err
}

// --- SQL Server --------------------------------------------------------------

func (db *MSSQL) editorSession() *editorSession {
	return editorSessionFor(db, func() *editorSession {
		return newEditorSession(func(ctx context.Context, database string) (*sql.Conn, func(), error) {
			use := ""
			if database != db.CurrentDatabase {
				use = "USE " + quoteMSSQLIdentifier(database)
			}
			return reserveEditorConn(ctx, db.Connection, use)
		}, mssqlTransactionOpen)
	})
}

func (db *MSSQL) editorDatabase(database string) string {
	if database == "" {
		return db.CurrentDatabase
	}
	return database
}

func (db *MSSQL) EditorStreamQuery(ctx context.Context, database, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error) {
	return db.editorSession().streamQuery(ctx, db.editorDatabase(database), mssqlEditorBatch(query), maxRows, onBatch)
}

func (db *MSSQL) EditorExec(ctx context.Context, database, query string) (string, error) {
	return db.editorSession().exec(ctx, db.editorDatabase(database), mssqlEditorBatch(query))
}

// mssqlEditorBatch keeps go-mssqldb from sending a one-word query such as
// COMMIT as a stored-procedure RPC: any newline makes it a plain batch, and a
// batch that names a procedure first still executes it.
func mssqlEditorBatch(query string) string {
	return query + "\n"
}

func (db *MSSQL) EditorTransactionOpen() bool { return transactionOpenFor(db) }

func (db *MSSQL) CloseEditorSession() error { return closeEditorSessionFor(db) }

// mssqlTransactionOpen counts a doomed transaction (XACT_STATE() = -1) as
// open too: it holds its locks until ROLLBACK.
func mssqlTransactionOpen(ctx context.Context, conn *sql.Conn) (bool, error) {
	var count int
	err := conn.QueryRowContext(ctx, "SELECT @@TRANCOUNT").Scan(&count)
	return count > 0, err
}

// --- SQLite ------------------------------------------------------------------

// SQLite pools hold one connection, so the session cannot reserve it without
// starving the rest of the app; it shares it instead.
func (db *SQLite) editorSession() *editorSession {
	return editorSessionFor(db, func() *editorSession {
		session := newEditorSession(func(ctx context.Context, _ string) (*sql.Conn, func(), error) {
			return reserveEditorConn(ctx, db.Connection, "")
		}, sqliteTransactionOpen)
		session.shared = true
		return session
	})
}

func (db *SQLite) EditorStreamQuery(ctx context.Context, _, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error) {
	return db.editorSession().streamQuery(ctx, "", query, maxRows, onBatch)
}

func (db *SQLite) EditorExec(ctx context.Context, _, query string) (string, error) {
	return db.editorSession().exec(ctx, "", query)
}

func (db *SQLite) EditorTransactionOpen() bool { return transactionOpenFor(db) }

func (db *SQLite) CloseEditorSession() error { return closeEditorSessionFor(db) }

// sqliteTransactionOpen uses BEGIN as the probe: SQLite rejects a nested
// BEGIN, and a deferred BEGIN outside a transaction takes no locks, so it is
// rolled back immediately without side effects.
func sqliteTransactionOpen(ctx context.Context, conn *sql.Conn) (bool, error) {
	_, err := conn.ExecContext(ctx, "BEGIN")
	if err == nil {
		_, err = conn.ExecContext(ctx, "ROLLBACK")
		return false, err
	}
	if strings.Contains(err.Error(), "within a transaction") {
		return true, nil
	}
	return false, err
}

// --- ClickHouse --------------------------------------------------------------

// ClickHouse transactions are experimental and have no state query, so the
// session only guarantees BEGIN/COMMIT reach the same connection.
func (db *ClickHouse) editorSession() *editorSession {
	return editorSessionFor(db, func() *editorSession {
		session := newEditorSession(func(ctx context.Context, _ string) (*sql.Conn, func(), error) {
			return reserveEditorConn(ctx, db.Connection, "")
		}, func(context.Context, *sql.Conn) (bool, error) { return false, nil })
		session.scanRow = scanClickHouseStreamRow
		return session
	})
}

func (db *ClickHouse) EditorStreamQuery(ctx context.Context, _, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error) {
	return db.editorSession().streamQuery(ctx, "", query, maxRows, onBatch)
}

func (db *ClickHouse) EditorExec(ctx context.Context, _, query string) (string, error) {
	return db.editorSession().exec(ctx, "", query)
}

func (db *ClickHouse) EditorTransactionOpen() bool { return transactionOpenFor(db) }

func (db *ClickHouse) CloseEditorSession() error { return closeEditorSessionFor(db) }
