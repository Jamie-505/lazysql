package drivers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// EditorSessionDriver is an optional capability: SQL editor statements run on
// one reserved database session instead of whichever pooled connection is
// free. Transaction control (BEGIN, COMMIT, ROLLBACK) then spans separate
// editor runs, and an open transaction can never leak back into the pool.
type EditorSessionDriver interface {
	EditorStreamQuery(ctx context.Context, database, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error)
	EditorExec(ctx context.Context, database, query string) (string, error)
	// EditorTransactionOpen reports whether the editor session is inside an
	// explicit (possibly aborted) transaction.
	EditorTransactionOpen() bool
	// CloseEditorSession discards the session; the server rolls back any open
	// transaction.
	CloseEditorSession() error
}

// ErrEditorTransactionLost reports that the editor session's connection
// failed while a transaction was open, so the server rolled it back.
var ErrEditorTransactionLost = errors.New("editor connection lost; the open transaction was rolled back")

const editorTransactionProbeTimeout = 5 * time.Second

// openEditorConn reserves a connection for database. release, when non-nil,
// frees resources the connection depends on (such as a per-database pool)
// after the connection itself is closed.
type openEditorConn func(ctx context.Context, database string) (conn *sql.Conn, release func(), err error)

// probeEditorTransaction reports whether conn is inside a transaction.
type probeEditorTransaction func(ctx context.Context, conn *sql.Conn) (bool, error)

type editorSession struct {
	mu       sync.Mutex
	conn     *sql.Conn
	release  func()
	database string
	txOpen   atomic.Bool

	open  openEditorConn
	probe probeEditorTransaction
	// scanRow overrides the default row scanner for streamed results.
	scanRow func(*sql.Rows, int) ([]string, error)
	// shared is set for single-connection pools (SQLite): holding the only
	// connection would starve every other operation, and the pool already
	// guarantees the editor always gets the same connection back.
	shared bool
}

var liveEditorSessions sync.Map // *editorSession -> struct{}

func newEditorSession(open openEditorConn, probe probeEditorTransaction) *editorSession {
	return &editorSession{open: open, probe: probe}
}

// EditorTransactionsOpen reports whether any editor session in this process
// has an open transaction, so quitting would roll it back.
func EditorTransactionsOpen() bool {
	open := false
	liveEditorSessions.Range(func(key, _ any) bool {
		open = key.(*editorSession).transactionOpen()
		return !open
	})
	return open
}

func (s *editorSession) transactionOpen() bool {
	return s.txOpen.Load()
}

// run executes fn on the reserved connection for database and then refreshes
// the transaction state.
func (s *editorSession) run(ctx context.Context, database string, fn func(editorConnection) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.acquire(ctx, database); err != nil {
		return err
	}
	err := fn(s.conn)
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		hadTx := s.txOpen.Load()
		s.discard()
		if hadTx {
			return errors.Join(err, ErrEditorTransactionLost)
		}
		return err
	}

	// The statement context may already be canceled; probe independently.
	probeCtx, cancel := context.WithTimeout(context.Background(), editorTransactionProbeTimeout)
	defer cancel()
	inTx, probeErr := s.probe(probeCtx, s.conn)
	if probeErr != nil {
		hadTx := s.txOpen.Load()
		s.discard()
		if hadTx {
			return errors.Join(err, ErrEditorTransactionLost)
		}
		return err
	}
	s.txOpen.Store(inTx)
	if s.shared {
		s.releaseConn()
	}
	return err
}

func (s *editorSession) acquire(ctx context.Context, database string) error {
	if s.conn != nil && s.database == database {
		return nil
	}
	if s.database != database && s.txOpen.Load() {
		return fmt.Errorf("a transaction is open on database %q; COMMIT or ROLLBACK it before querying %q", s.database, database)
	}
	if s.conn != nil {
		s.discard()
	}

	conn, release, err := s.open(ctx, database)
	if err != nil {
		return err
	}
	s.conn = conn
	s.release = release
	s.database = database
	return nil
}

// releaseConn returns a shared session's connection to its pool, keeping the
// session state (the pool holds a single connection).
func (s *editorSession) releaseConn() {
	_ = s.conn.Close()
	s.conn = nil
}

// discard closes the reserved connection instead of returning it to the pool,
// so session state (including any transaction) dies with it.
func (s *editorSession) discard() {
	if s.conn != nil {
		_ = s.conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = s.conn.Close()
		s.conn = nil
	}
	if s.release != nil {
		s.release()
		s.release = nil
	}
	s.database = ""
	s.txOpen.Store(false)
}

func (s *editorSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shared && s.txOpen.Load() {
		// The pooled connection survives, so end the transaction explicitly.
		s.txOpen.Store(false)
		return s.rollbackShared()
	}
	s.discard()
	return nil
}

func (s *editorSession) rollbackShared() error {
	ctx, cancel := context.WithTimeout(context.Background(), editorTransactionProbeTimeout)
	defer cancel()
	conn, release, err := s.open(ctx, s.database)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
		if release != nil {
			release()
		}
	}()
	_, err = conn.ExecContext(ctx, "ROLLBACK")
	return err
}

// driverEditorSessions holds each driver's lazily created editor session.
// Keeping it outside the driver structs leaves them copyable.
var driverEditorSessions sync.Map // driver pointer -> *editorSession

func editorSessionFor(owner any, create func() *editorSession) *editorSession {
	if session, ok := driverEditorSessions.Load(owner); ok {
		return session.(*editorSession)
	}
	session, loaded := driverEditorSessions.LoadOrStore(owner, create())
	if !loaded {
		liveEditorSessions.Store(session, struct{}{})
	}
	return session.(*editorSession)
}

func existingEditorSession(owner any) *editorSession {
	if session, ok := driverEditorSessions.Load(owner); ok {
		return session.(*editorSession)
	}
	return nil
}

func (s *editorSession) streamQuery(ctx context.Context, database, query string, maxRows int, onBatch func(QueryBatch) error) (QueryStreamResult, error) {
	var result QueryStreamResult
	err := s.run(ctx, database, func(conn editorConnection) error {
		var err error
		scanRow := s.scanRow
		if scanRow == nil {
			scanRow = scanQueryRow
		}
		result, err = streamQueryWithScanner(ctx, conn, query, maxRows, onBatch, scanRow)
		return err
	})
	return result, err
}

func (s *editorSession) exec(ctx context.Context, database, query string) (string, error) {
	var message string
	err := s.run(ctx, database, func(conn editorConnection) error {
		res, err := conn.ExecContext(contextOrBackground(ctx), query)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		message = fmt.Sprintf("%d rows affected", affected)
		return nil
	})
	return message, err
}

func transactionOpenFor(owner any) bool {
	session := existingEditorSession(owner)
	return session != nil && session.transactionOpen()
}

func closeEditorSessionFor(owner any) error {
	if session := existingEditorSession(owner); session != nil {
		return session.close()
	}
	return nil
}
