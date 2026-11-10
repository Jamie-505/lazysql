// Package explain turns database query plans into a tree that can be shown
// to the user.
//
// The PostgreSQL JSON plan parser is adapted from vi-sql
// (https://github.com/kopecmaciej/vi-sql, internal/sql/explain.go),
// Copyright its authors, licensed under the Apache License 2.0.
package explain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanNode is one operation of a query plan.
type PlanNode struct {
	NodeType     string
	RelationName string
	Alias        string
	IndexName    string
	JoinType     string
	Condition    string
	StartupCost  float64
	TotalCost    float64
	PlanRows     int64
	ActualRows   int64
	ActualLoops  int64
	ActualMs     float64
	Children     []*PlanNode
}

// Plan is a parsed query plan. PlanningMs and ExecutionMs are only set when
// the query was actually executed (EXPLAIN ANALYZE).
type Plan struct {
	Root        *PlanNode
	PlanningMs  float64
	ExecutionMs float64
}

// rawPlanNode mirrors the PostgreSQL JSON plan node. PostgreSQL writes the
// row and loop counts as floats.
type rawPlanNode struct {
	NodeType     string         `json:"Node Type"`
	RelationName string         `json:"Relation Name"`
	Alias        string         `json:"Alias"`
	IndexName    string         `json:"Index Name"`
	JoinType     string         `json:"Join Type"`
	Filter       string         `json:"Filter"`
	IndexCond    string         `json:"Index Cond"`
	HashCond     string         `json:"Hash Cond"`
	MergeCond    string         `json:"Merge Cond"`
	JoinFilter   string         `json:"Join Filter"`
	StartupCost  float64        `json:"Startup Cost"`
	TotalCost    float64        `json:"Total Cost"`
	PlanRows     float64        `json:"Plan Rows"`
	ActualRows   float64        `json:"Actual Rows"`
	ActualLoops  float64        `json:"Actual Loops"`
	ActualMs     float64        `json:"Actual Total Time"`
	Plans        []*rawPlanNode `json:"Plans"`
}

type rawTopLevel struct {
	Plan        *rawPlanNode `json:"Plan"`
	PlanningMs  float64      `json:"Planning Time"`
	ExecutionMs float64      `json:"Execution Time"`
}

func convertNode(raw *rawPlanNode) *PlanNode {
	node := &PlanNode{
		NodeType:     raw.NodeType,
		RelationName: raw.RelationName,
		Alias:        raw.Alias,
		IndexName:    raw.IndexName,
		JoinType:     raw.JoinType,
		StartupCost:  raw.StartupCost,
		TotalCost:    raw.TotalCost,
		PlanRows:     int64(raw.PlanRows),
		ActualRows:   int64(raw.ActualRows),
		ActualLoops:  int64(raw.ActualLoops),
		ActualMs:     raw.ActualMs,
	}
	for _, condition := range []string{raw.IndexCond, raw.HashCond, raw.MergeCond, raw.JoinFilter, raw.Filter} {
		if condition != "" {
			node.Condition = condition
			break
		}
	}
	for _, child := range raw.Plans {
		if child != nil {
			node.Children = append(node.Children, convertNode(child))
		}
	}
	return node
}

// ParsePostgresPlan parses the output of EXPLAIN (FORMAT JSON), with or
// without ANALYZE.
func ParsePostgresPlan(jsonText string) (*Plan, error) {
	var top []rawTopLevel
	if err := json.Unmarshal([]byte(jsonText), &top); err != nil {
		return nil, fmt.Errorf("parsing query plan: %w", err)
	}
	if len(top) == 0 || top[0].Plan == nil {
		return nil, fmt.Errorf("parsing query plan: no plan in the output")
	}
	return &Plan{
		Root:        convertNode(top[0].Plan),
		PlanningMs:  top[0].PlanningMs,
		ExecutionMs: top[0].ExecutionMs,
	}, nil
}

// Label is the one-line description of a node. analyzed adds the measured
// rows, loops and time next to the planner's estimates.
func (node *PlanNode) Label(analyzed bool) string {
	var label strings.Builder
	if node.JoinType != "" && !strings.Contains(node.NodeType, node.JoinType) {
		label.WriteString(node.JoinType + " ")
	}
	label.WriteString(node.NodeType)
	if node.IndexName != "" {
		label.WriteString(" using " + node.IndexName)
	}
	if node.RelationName != "" {
		label.WriteString(" on " + node.RelationName)
		if node.Alias != "" && node.Alias != node.RelationName {
			label.WriteString(" " + node.Alias)
		}
	}
	fmt.Fprintf(&label, "  cost=%.2f..%.2f", node.StartupCost, node.TotalCost)
	if analyzed {
		fmt.Fprintf(&label, "  rows=%d (est %d)  loops=%d  time=%.3fms", node.ActualRows, node.PlanRows, node.ActualLoops, node.ActualMs)
	} else {
		fmt.Fprintf(&label, "  rows=%d", node.PlanRows)
	}
	if node.Condition != "" {
		label.WriteString("  " + node.Condition)
	}
	return label.String()
}

// Text renders the plan as an indented outline, for copying.
func (plan *Plan) Text(analyzed bool) string {
	var out strings.Builder
	var walk func(node *PlanNode, depth int)
	walk = func(node *PlanNode, depth int) {
		out.WriteString(strings.Repeat("  ", depth))
		if depth > 0 {
			out.WriteString("-> ")
		}
		out.WriteString(node.Label(analyzed))
		out.WriteString("\n")
		for _, child := range node.Children {
			walk(child, depth+1)
		}
	}
	walk(plan.Root, 0)
	if analyzed {
		fmt.Fprintf(&out, "Planning: %.3f ms  Execution: %.3f ms\n", plan.PlanningMs, plan.ExecutionMs)
	}
	return out.String()
}
