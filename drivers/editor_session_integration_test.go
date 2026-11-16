package drivers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type editorSessionCase struct {
	name   string
	driver interface {
		Driver
		EditorSessionDriver
	}
	url    string
	begin  string
	commit string
	// shared drivers run the editor on the app's only connection, so
	// uncommitted changes are visible to every caller.
	shared bool
	// readsBlock is set where plain reads wait on another transaction's row
	// locks (SQL Server without READ_COMMITTED_SNAPSHOT).
	readsBlock bool
}

// MySQL and SQL Server need LAZYSQL_TEST_MYSQL_URL / LAZYSQL_TEST_MSSQL_URL
// pointing at disposable instances; SQLite always runs on a temp file.
func editorSessionCases(t *testing.T) []editorSessionCase {
	t.Helper()
	sqlitePath := filepath.Join(t.TempDir(), "session.db")
	return []editorSessionCase{
		{name: "SQLITE", driver: &SQLite{}, url: "file:" + sqlitePath, begin: "BEGIN", commit: "COMMIT", shared: true},
		{name: "MYSQL", driver: &MySQL{}, url: os.Getenv("LAZYSQL_TEST_MYSQL_URL"), begin: "BEGIN", commit: "COMMIT"},
		{name: "MSSQL", driver: &MSSQL{}, url: os.Getenv("LAZYSQL_TEST_MSSQL_URL"), begin: "BEGIN TRANSACTION", commit: "COMMIT", readsBlock: true},
	}
}

func TestEditorSessionTransactionsAcrossDrivers(t *testing.T) {
	for _, tc := range editorSessionCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			if tc.url == "" {
				t.Skip("disposable database URL not set")
			}
			ctx := context.Background()
			db := tc.driver
			if err := db.Connect(ctx, tc.url); err != nil {
				t.Fatal(err)
			}
			table := fmt.Sprintf("editor_session_%d", time.Now().UnixNano())
			mustExec := func(query string) {
				t.Helper()
				if _, err := db.ExecuteDMLStatement(ctx, "", query); err != nil {
					t.Fatalf("%s: %v", query, err)
				}
			}
			mustExec("CREATE TABLE " + table + " (id INT PRIMARY KEY, v VARCHAR(10))")
			mustExec("INSERT INTO " + table + " VALUES (1, 'a')")
			mustExec("INSERT INTO " + table + " VALUES (2, 'b')")
			t.Cleanup(func() {
				_ = db.CloseEditorSession()
				_, _ = db.ExecuteDMLStatement(context.Background(), "", "DROP TABLE "+table)
			})
			pooled := func() string {
				t.Helper()
				rows, _, err := db.ExecuteQuery(ctx, "", "SELECT v FROM "+table+" WHERE id = 1")
				if err != nil || len(rows) != 2 {
					t.Fatalf("pooled read: %v %v", rows, err)
				}
				return rows[1][0]
			}

			// BEGIN and COMMIT in separate runs reach the same transaction.
			if _, err := db.EditorExec(ctx, "", tc.begin); err != nil {
				t.Fatal(err)
			}
			if !db.EditorTransactionOpen() || !EditorTransactionsOpen() {
				t.Fatal("transaction not reported open after BEGIN")
			}
			if msg, err := db.EditorExec(ctx, "", "UPDATE "+table+" SET v = 'z'"); err != nil || msg != "2 rows affected" {
				t.Fatalf("update = %q, %v", msg, err)
			}
			if !tc.shared && !tc.readsBlock {
				if got := pooled(); got != "a" {
					t.Fatalf("uncommitted change visible to pool: %q", got)
				}
			}
			if _, err := db.EditorExec(ctx, "", tc.commit); err != nil {
				t.Fatal(err)
			}
			if db.EditorTransactionOpen() {
				t.Fatal("transaction reported open after COMMIT")
			}
			if got := pooled(); got != "z" {
				t.Fatalf("committed change not visible: %q", got)
			}

			// Closing the session rolls back and releases locks.
			if _, err := db.EditorExec(ctx, "", tc.begin); err != nil {
				t.Fatal(err)
			}
			if _, err := db.EditorExec(ctx, "", "UPDATE "+table+" SET v = 'q'"); err != nil {
				t.Fatal(err)
			}
			if err := db.CloseEditorSession(); err != nil {
				t.Fatal(err)
			}
			if db.EditorTransactionOpen() || EditorTransactionsOpen() {
				t.Fatal("transaction reported open after close")
			}
			lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if _, err := db.ExecuteDMLStatement(lockCtx, "", "UPDATE "+table+" SET v = 'p' WHERE id = 2"); err != nil {
				t.Fatalf("row still locked after session close: %v", err)
			}
			if got := pooled(); got != "z" {
				t.Fatalf("rolled-back change visible: %q", got)
			}

			// The session reopens transparently and streams results.
			var rows [][]string
			result, err := db.EditorStreamQuery(ctx, "", "SELECT v FROM "+table+" ORDER BY id", 0, func(batch QueryBatch) error {
				rows = append(rows, batch.Rows...)
				return nil
			})
			if err != nil || result.Rows != 2 {
				t.Fatalf("stream after reopen: %+v %v", result, err)
			}
		})
	}
}

// ClickHouse has no usable transactions; the session must still run queries
// with the ClickHouse row scanner. Needs LAZYSQL_CLICKHOUSE_URL.
func TestClickHouseEditorSessionStreams(t *testing.T) {
	url := os.Getenv("LAZYSQL_CLICKHOUSE_URL")
	if url == "" {
		t.Skip("LAZYSQL_CLICKHOUSE_URL not set")
	}
	db := &ClickHouse{}
	ctx := context.Background()
	if err := db.Connect(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.CloseEditorSession() })

	var rows [][]string
	result, err := db.EditorStreamQuery(ctx, "", "SELECT number, NULL FROM numbers(3)", 0, func(batch QueryBatch) error {
		rows = append(rows, batch.Rows...)
		return nil
	})
	if err != nil || result.Rows != 3 || rows[0][1] != "NULL" {
		t.Fatalf("stream = %+v %v %v", result, rows, err)
	}
	if _, err := db.EditorExec(ctx, "", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if db.EditorTransactionOpen() {
		t.Fatal("ClickHouse reported an open transaction")
	}
}
