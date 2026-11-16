package drivers

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

func (db *Postgres) editorSession() *editorSession {
	return editorSessionFor(db, func() *editorSession {
		return newEditorSession(db.openEditorConn, postgresTransactionOpen)
	})
}

// openEditorConn reserves a connection; a non-login database gets its own
// pool, closed with the session.
func (db *Postgres) openEditorConn(ctx context.Context, database string) (*sql.Conn, func(), error) {
	pool, owned, err := db.connectionFor(ctx, database)
	if err != nil {
		return nil, nil, err
	}
	var release func()
	if owned {
		release = func() { _ = pool.Close() }
	}
	conn, err := pool.Conn(contextOrBackground(ctx))
	if err != nil {
		if release != nil {
			release()
		}
		return nil, nil, err
	}
	return conn, release, nil
}

func (db *Postgres) editorDatabase(database string) string {
	if database == "" {
		return db.CurrentDatabase
	}
	return database
}

func (db *Postgres) EditorStreamQuery(ctx context.Context, database, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error) {
	return db.editorSession().streamQuery(ctx, db.editorDatabase(database), query, maxRows, onBatch)
}

func (db *Postgres) EditorExec(ctx context.Context, database, query string) (string, error) {
	return db.editorSession().exec(ctx, db.editorDatabase(database), query)
}

func (db *Postgres) EditorTransactionOpen() bool { return transactionOpenFor(db) }

func (db *Postgres) CloseEditorSession() error { return closeEditorSessionFor(db) }

// postgresTransactionOpen relies on now() being the transaction start time:
// outside an explicit transaction every statement is its own transaction, so
// now() equals statement_timestamp(). An aborted transaction rejects the
// probe with 25P02 but is still open until ROLLBACK.
func postgresTransactionOpen(ctx context.Context, conn *sql.Conn) (bool, error) {
	var open bool
	err := conn.QueryRowContext(ctx, "SELECT now() <> statement_timestamp()").Scan(&open)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "25P02" {
		return true, nil
	}
	return open, err
}
