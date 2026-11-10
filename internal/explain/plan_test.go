package explain

import (
	"strings"
	"testing"
)

const analyzedPlan = `[{"Plan": {"Node Type": "Hash Join", "Join Type": "Inner", "Startup Cost": 1.5, "Total Cost": 30.25,
  "Plan Rows": 100, "Actual Rows": 90.0, "Actual Loops": 1, "Actual Total Time": 0.42, "Hash Cond": "(o.user_id = u.id)",
  "Plans": [
    {"Node Type": "Seq Scan", "Relation Name": "orders", "Alias": "o", "Startup Cost": 0, "Total Cost": 20, "Plan Rows": 1000, "Actual Rows": 950, "Actual Loops": 1, "Actual Total Time": 0.2},
    {"Node Type": "Index Scan", "Relation Name": "users", "Alias": "users", "Index Name": "users_pkey", "Index Cond": "(id = 1)", "Startup Cost": 0.1, "Total Cost": 8.3, "Plan Rows": 1, "Actual Rows": 1, "Actual Loops": 1, "Actual Total Time": 0.01}
  ]}, "Planning Time": 0.11, "Execution Time": 0.57}]`

func TestParsePostgresPlan(t *testing.T) {
	plan, err := ParsePostgresPlan(analyzedPlan)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanningMs != 0.11 || plan.ExecutionMs != 0.57 {
		t.Errorf("timings = %v/%v", plan.PlanningMs, plan.ExecutionMs)
	}
	root := plan.Root
	if root.NodeType != "Hash Join" || len(root.Children) != 2 || root.Condition != "(o.user_id = u.id)" {
		t.Fatalf("root = %+v", root)
	}
	if root.ActualRows != 90 || root.PlanRows != 100 {
		t.Errorf("rows = %d/%d, want 90/100", root.ActualRows, root.PlanRows)
	}

	scan := root.Children[1]
	label := scan.Label(false)
	for _, want := range []string{"Index Scan using users_pkey on users", "cost=0.10..8.30", "rows=1", "(id = 1)"} {
		if !strings.Contains(label, want) {
			t.Errorf("label %q does not contain %q", label, want)
		}
	}
	if strings.Contains(label, "users users") {
		t.Errorf("label %q repeats an alias equal to the relation", label)
	}
	if got := root.Children[0].Label(true); !strings.Contains(got, "on orders o") || !strings.Contains(got, "rows=950 (est 1000)") || !strings.Contains(got, "time=0.200ms") {
		t.Errorf("analyzed label = %q", got)
	}
	if got := root.Label(false); !strings.HasPrefix(got, "Inner Hash Join") {
		t.Errorf("join label = %q", got)
	}
}

func TestPlanText(t *testing.T) {
	plan, err := ParsePostgresPlan(analyzedPlan)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(plan.Text(true), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("text has %d lines, want 4:\n%s", len(lines), plan.Text(true))
	}
	if !strings.HasPrefix(lines[1], "  -> Seq Scan") || !strings.HasPrefix(lines[3], "Planning: 0.110 ms") {
		t.Errorf("unexpected outline:\n%s", plan.Text(true))
	}
	if strings.Contains(plan.Text(false), "Planning") {
		t.Error("plan-only text shows timings")
	}
}

func TestParsePostgresPlanErrors(t *testing.T) {
	for _, input := range []string{"", "not json", "[]", `[{"Planning Time": 1}]`} {
		if _, err := ParsePostgresPlan(input); err == nil {
			t.Errorf("ParsePostgresPlan(%q): error = nil", input)
		}
	}
}
