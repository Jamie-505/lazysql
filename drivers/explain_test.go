package drivers

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStripExplainPrefix(t *testing.T) {
	tests := []struct {
		query   string
		bare    string
		analyze bool
	}{
		{"SELECT 1;", "SELECT 1", false},
		{"  explain select 1 ;\n", "select 1", false},
		{"EXPLAIN ANALYZE SELECT 1", "SELECT 1", true},
		{"EXPLAIN (ANALYZE, FORMAT JSON) SELECT 1", "SELECT 1", true},
		{"EXPLAIN (FORMAT JSON) SELECT 1", "SELECT 1", false},
		{"EXPLAIN QUERY PLAN SELECT 1", "SELECT 1", false},
		{"EXPLAIN FORMAT=TREE SELECT 1", "SELECT 1", false},
		{"EXPLAIN\nSELECT 1", "SELECT 1", false},
		{"SELECT explain FROM t", "SELECT explain FROM t", false},
		{"explainer_view", "explainer_view", false},
	}
	for _, tt := range tests {
		bare, analyze := StripExplainPrefix(tt.query)
		if bare != tt.bare || analyze != tt.analyze {
			t.Errorf("StripExplainPrefix(%q) = %q, %v; want %q, %v", tt.query, bare, analyze, tt.bare, tt.analyze)
		}
	}
}

func newExplainMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func TestExplainQueryPostgres(t *testing.T) {
	db, mock := newExplainMock(t)
	pg := &Postgres{Connection: db, Provider: DriverPostgres}

	mock.ExpectQuery("EXPLAIN (FORMAT JSON) SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"QUERY PLAN"}).AddRow(`[{"Plan": {}}]`))
	result, err := ExplainQuery(context.Background(), pg, "", "EXPLAIN SELECT 1;", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Format != ExplainFormatPostgresJSON || result.Text != `[{"Plan": {}}]` || result.Analyzed {
		t.Errorf("result = %+v", result)
	}

	mock.ExpectQuery("EXPLAIN (ANALYZE, FORMAT JSON) SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"QUERY PLAN"}).AddRow(`[]`))
	result, err = ExplainQuery(context.Background(), pg, "", "SELECT 1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analyzed {
		t.Error("Analyzed = false after an ANALYZE run")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestExplainQueryMySQLFallsBackToTabularPlan(t *testing.T) {
	db, mock := newExplainMock(t)
	mysql := &MySQL{Connection: db, Provider: DriverMySQL}

	mock.ExpectQuery("EXPLAIN FORMAT=TREE SELECT 1").WillReturnError(errors.New("syntax error near FORMAT"))
	mock.ExpectQuery("EXPLAIN SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "select_type", "table"}).AddRow("1", "SIMPLE", "users"))

	result, err := ExplainQuery(context.Background(), mysql, "", "SELECT 1", false)
	if err != nil {
		t.Fatal(err)
	}
	want := "id  select_type  table\n1   SIMPLE       users"
	if result.Text != want {
		t.Errorf("text = %q, want %q", result.Text, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestExplainQuerySQLiteIndentsPlanTree(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, statement := range []string{
		"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)",
		"CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER)",
	} {
		if _, err := conn.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	sqlite := &SQLite{Connection: conn, Provider: DriverSqlite}

	result, err := ExplainQuery(context.Background(), sqlite, "", "SELECT * FROM orders o JOIN users u ON u.id = o.user_id", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "SCAN") || !strings.Contains(result.Text, "SEARCH") {
		t.Errorf("plan text = %q, want a scan and a search", result.Text)
	}

	if _, err := ExplainQuery(context.Background(), sqlite, "", "SELECT 1", true); err == nil {
		t.Error("ANALYZE on SQLite: error = nil")
	}
	if _, err := ExplainQuery(context.Background(), sqlite, "", "EXPLAIN ;", false); err == nil {
		t.Error("empty query: error = nil")
	}
}

func TestSQLiteQueryPlanTextNestsChildren(t *testing.T) {
	rows := [][]string{
		{"id", "parent", "notused", "detail"},
		{"2", "0", "0", "CO-ROUTINE sub"},
		{"5", "2", "0", "SCAN t"},
		{"9", "0", "0", "SCAN sub"},
	}
	want := "CO-ROUTINE sub\n  SCAN t\nSCAN sub"
	if got := sqliteQueryPlanText(rows); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestExplainQueryMSSQLUsesShowplanOnReservedConnection(t *testing.T) {
	db, mock := newExplainMock(t)
	mssql := &MSSQL{Connection: db, Provider: DriverMSSQL, CurrentDatabase: "master"}

	mock.ExpectExec("USE [shop]").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SET SHOWPLAN_TEXT ON").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(
		sqlmock.NewRows([]string{"StmtText"}).AddRow("SELECT 1"),
		sqlmock.NewRows([]string{"StmtText"}).AddRow("  |--Constant Scan"),
	)

	result, err := ExplainQuery(context.Background(), mssql, "shop", "SELECT 1", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "SELECT 1\n  |--Constant Scan" {
		t.Errorf("text = %q", result.Text)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
