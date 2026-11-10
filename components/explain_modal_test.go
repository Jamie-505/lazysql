package components

import (
	"context"
	"strings"
	"testing"

	"github.com/jorgerojas26/lazysql/drivers"
)

func TestExplainAnalyzeBlockReason(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		provider string
		allowed  bool
	}{
		{"select", "SELECT * FROM users", drivers.DriverPostgres, true},
		{"explain prefix", "EXPLAIN ANALYZE SELECT * FROM users;", drivers.DriverPostgres, true},
		{"cte select", "WITH a AS (SELECT 1) SELECT * FROM a", drivers.DriverMySQL, true},
		{"keyword inside string", "SELECT 'delete me' FROM users", drivers.DriverPostgres, true},
		{"keyword inside comment", "SELECT 1 -- then update\n", drivers.DriverPostgres, true},
		{"update", "UPDATE users SET name = 'x'", drivers.DriverPostgres, false},
		{"delete", "DELETE FROM users", drivers.DriverMySQL, false},
		{"writing cte", "WITH gone AS (DELETE FROM users RETURNING *) SELECT * FROM gone", drivers.DriverPostgres, false},
		{"select into", "SELECT * INTO backup FROM users", drivers.DriverPostgres, false},
		{"ddl", "DROP TABLE users", drivers.DriverPostgres, false},
		{"empty", "  ", drivers.DriverPostgres, false},
		{"sqlite", "SELECT 1", drivers.DriverSqlite, false},
		{"mssql", "SELECT 1", drivers.DriverMSSQL, false},
		{"clickhouse", "SELECT 1", drivers.DriverClickHouse, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := explainAnalyzeBlockReason(tt.query, tt.provider)
			if (reason == "") != tt.allowed {
				t.Fatalf("reason = %q, allowed want %v", reason, tt.allowed)
			}
		})
	}
}

func newTestExplainModal(blocked string) *ExplainModal {
	return NewExplainModal(func(context.Context, bool) (drivers.ExplainResult, error) {
		return drivers.ExplainResult{}, nil
	}, blocked, nil)
}

func TestExplainModalRendersPostgresPlanAsTree(t *testing.T) {
	modal := newTestExplainModal("")
	modal.Render(drivers.ExplainResult{
		Format:   drivers.ExplainFormatPostgresJSON,
		Analyzed: true,
		Text: `[{"Plan": {"Node Type": "Limit", "Total Cost": 1.5, "Plan Rows": 10, "Actual Rows": 10, "Actual Loops": 1,
			"Plans": [{"Node Type": "Seq Scan", "Relation Name": "users", "Total Cost": 1.2, "Plan Rows": 50}]},
			"Planning Time": 0.25, "Execution Time": 1.5}]`,
	})

	if page, _ := modal.body.GetFrontPage(); page != explainPageTree {
		t.Fatalf("front page = %q, want the tree", page)
	}
	root := modal.tree.GetRoot()
	if root == nil || !strings.HasPrefix(root.GetText(), "Limit") || len(root.GetChildren()) != 1 {
		t.Fatalf("tree root = %+v", root)
	}
	if child := root.GetChildren()[0].GetText(); !strings.Contains(child, "Seq Scan on users") {
		t.Errorf("child = %q", child)
	}
	status := modal.status.GetText(true)
	if !strings.Contains(status, "Execution 1.500 ms") || !strings.Contains(status, "a plan only") {
		t.Errorf("status = %q", status)
	}
	if !strings.Contains(modal.planText, "-> Seq Scan on users") {
		t.Errorf("copy text = %q", modal.planText)
	}
	if !strings.Contains(modal.panel.GetTitle(), "ANALYZE") {
		t.Errorf("title = %q, want it to mark the measured plan", modal.panel.GetTitle())
	}
}

func TestExplainModalRendersTextPlan(t *testing.T) {
	modal := newTestExplainModal("ANALYZE is not available for sqlite3")
	modal.Render(drivers.ExplainResult{Text: "SCAN users\n  SEARCH orders USING INDEX idx"})

	if page, _ := modal.body.GetFrontPage(); page != explainPageText {
		t.Fatalf("front page = %q, want the text view", page)
	}
	if got := modal.text.GetText(true); !strings.Contains(got, "SEARCH orders USING INDEX idx") {
		t.Errorf("text = %q", got)
	}
	if status := modal.status.GetText(true); !strings.Contains(status, "analyze unavailable") {
		t.Errorf("status = %q", status)
	}

	// A blocked ANALYZE explains itself instead of running.
	modal.toggleAnalyze()
	if status := modal.status.GetText(true); !strings.Contains(status, "not available for sqlite3") {
		t.Errorf("status after blocked toggle = %q", status)
	}
	if modal.loading {
		t.Error("blocked ANALYZE started a run")
	}
}

func TestExplainModalShowsRawTextWhenPlanDoesNotParse(t *testing.T) {
	modal := newTestExplainModal("")
	modal.Render(drivers.ExplainResult{Format: drivers.ExplainFormatPostgresJSON, Text: "not json"})
	if page, _ := modal.body.GetFrontPage(); page != explainPageText {
		t.Fatalf("front page = %q, want the raw text", page)
	}
	if got := modal.text.GetText(true); !strings.Contains(got, "not json") {
		t.Errorf("text = %q", got)
	}
}
