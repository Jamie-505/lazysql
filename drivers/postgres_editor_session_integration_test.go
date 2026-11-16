package drivers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Set LAZYSQL_TEST_POSTGRES_URL to a disposable instance.
func newPostgresEditorSessionTest(t *testing.T) (*Postgres, string) {
	t.Helper()
	url := os.Getenv("LAZYSQL_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("disposable database URL not set")
	}
	db := &Postgres{}
	ctx := context.Background()
	if err := db.Connect(ctx, url); err != nil {
		t.Fatal(err)
	}
	table := fmt.Sprintf("editor_session_%d", time.Now().UnixNano())
	if _, err := db.Connection.ExecContext(ctx, "CREATE TABLE "+table+" (id int PRIMARY KEY, v text); INSERT INTO "+table+" VALUES (1, 'a'), (2, 'b')"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.CloseEditorSession()
		_, _ = db.Connection.ExecContext(context.Background(), "DROP TABLE "+table)
		_ = db.Connection.Close()
	})
	return db, table
}

func pooledValue(t *testing.T, db *Postgres, table string) string {
	t.Helper()
	var v string
	if err := db.Connection.QueryRowContext(context.Background(), "SELECT v FROM "+table+" WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPostgresEditorSessionTransactionSpansRuns(t *testing.T) {
	db, table := newPostgresEditorSessionTest(t)
	ctx := context.Background()

	var rows [][]string
	result, err := db.EditorStreamQuery(ctx, "", "BEGIN;\nUPDATE "+table+" SET v = 'z' RETURNING id, v;", 0, func(batch QueryBatch) error {
		rows = append(rows, batch.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 2 || len(rows) != 2 {
		t.Fatalf("RETURNING rows = %d (%v), want 2", result.Rows, rows)
	}
	if !db.EditorTransactionOpen() || !EditorTransactionsOpen() {
		t.Fatal("transaction not reported open after BEGIN")
	}
	if got := pooledValue(t, db, table); got != "a" {
		t.Fatalf("uncommitted change visible to pool: %q", got)
	}

	if _, err := db.EditorExec(ctx, "", "COMMIT"); err != nil {
		t.Fatal(err)
	}
	if db.EditorTransactionOpen() {
		t.Fatal("transaction still reported open after COMMIT")
	}
	if got := pooledValue(t, db, table); got != "z" {
		t.Fatalf("committed change not visible: %q", got)
	}
}

func TestPostgresEditorSessionRollbackAndAbortedTransaction(t *testing.T) {
	db, table := newPostgresEditorSessionTest(t)
	ctx := context.Background()

	if msg, err := db.EditorExec(ctx, "", "BEGIN; UPDATE "+table+" SET v = 'z'"); err != nil || msg != "2 rows affected" {
		t.Fatalf("exec = %q, %v", msg, err)
	}
	if _, err := db.EditorExec(ctx, "", "SELECT 1/0"); err == nil {
		t.Fatal("division by zero succeeded")
	}
	if !db.EditorTransactionOpen() {
		t.Fatal("aborted transaction not reported open")
	}
	if _, err := db.EditorExec(ctx, "", "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if db.EditorTransactionOpen() {
		t.Fatal("transaction reported open after ROLLBACK")
	}
	if got := pooledValue(t, db, table); got != "a" {
		t.Fatalf("rolled-back change visible: %q", got)
	}
}

func TestPostgresEditorSessionCloseRollsBack(t *testing.T) {
	db, table := newPostgresEditorSessionTest(t)
	ctx := context.Background()

	if _, err := db.EditorExec(ctx, "", "BEGIN; UPDATE "+table+" SET v = 'z'"); err != nil {
		t.Fatal(err)
	}
	if err := db.CloseEditorSession(); err != nil {
		t.Fatal(err)
	}
	if db.EditorTransactionOpen() {
		t.Fatal("transaction reported open after close")
	}
	// The row lock is gone: a pooled update must not block.
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := db.Connection.ExecContext(lockCtx, "UPDATE "+table+" SET v = 'p' WHERE id = 1"); err != nil {
		t.Fatalf("row still locked after session close: %v", err)
	}
	// The session reopens transparently.
	if _, err := db.EditorExec(ctx, "", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresEditorSessionRefusesDatabaseSwitchInTransaction(t *testing.T) {
	db, _ := newPostgresEditorSessionTest(t)
	ctx := context.Background()

	if _, err := db.EditorExec(ctx, "", "BEGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EditorExec(ctx, "template1", "SELECT 1"); err == nil {
		t.Fatal("switched database with an open transaction")
	}
	if !db.EditorTransactionOpen() {
		t.Fatal("refused switch dropped the transaction")
	}
}
