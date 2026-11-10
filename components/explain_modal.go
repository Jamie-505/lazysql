package components

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/helpers/logger"
	"github.com/jorgerojas26/lazysql/internal/explain"
	"github.com/jorgerojas26/lazysql/lib"
)

const (
	explainPageTree = "tree"
	explainPageText = "text"
)

// explainFunc fetches a plan. It is a field so tests can render plans
// without a database.
type explainFunc func(ctx context.Context, analyze bool) (drivers.ExplainResult, error)

// ExplainModal is the Plan Viewer: it shows the plan of one editor query and
// can re-run it with ANALYZE.
type ExplainModal struct {
	*tview.Flex
	panel  *tview.Flex
	body   *tview.Pages
	tree   *tview.TreeView
	text   *tview.TextView
	status *tview.TextView

	explain explainFunc
	// analyzeBlocked is why ANALYZE may not run for this query, or empty.
	analyzeBlocked string
	analyzed       bool
	loading        bool
	generation     uint64
	cancel         context.CancelFunc
	// planText is the plan as plain text, for copying.
	planText string
	onClose  func()
}

// explainAnalyzeBlockReason returns why EXPLAIN ANALYZE must not run for the
// query, or "" when it may. ANALYZE executes the statement, so it is limited
// to statements that only read.
func explainAnalyzeBlockReason(query, provider string) string {
	if !drivers.SupportsExplainAnalyze(provider) {
		return "ANALYZE is not available for " + provider
	}
	bare, _ := drivers.StripExplainPrefix(query)
	tokens := sqlRoutingTokens(bare, provider)
	if len(tokens) == 0 {
		return "No query to analyze"
	}
	const blocked = "ANALYZE executes the query, so it is only available for read-only SELECT statements"
	switch tokens[0] {
	case "select", "with", "values", "table":
	default:
		return blocked
	}
	for _, token := range tokens {
		switch token {
		case "insert", "update", "delete", "merge", "replace", "into", "truncate",
			"create", "alter", "drop", "grant", "revoke", "call", "do", "copy", "lock":
			return blocked
		}
	}
	return ""
}

func NewExplainModal(explain explainFunc, analyzeBlocked string, onClose func()) *ExplainModal {
	modal := &ExplainModal{
		Flex:           tview.NewFlex(),
		panel:          tview.NewFlex().SetDirection(tview.FlexRow),
		body:           tview.NewPages(),
		tree:           tview.NewTreeView(),
		text:           tview.NewTextView().SetScrollable(true).SetWrap(false),
		status:         tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter),
		explain:        explain,
		analyzeBlocked: analyzeBlocked,
		onClose:        onClose,
	}

	modal.tree.SetGraphicsColor(app.Styles.GraphicsColor)
	modal.body.AddPage(explainPageTree, modal.tree, true, false)
	modal.body.AddPage(explainPageText, modal.text, true, true)

	// A Flex has a transparent Box; give the panel a solid surface so the
	// page underneath does not show through.
	modal.panel.Box = tview.NewBox()
	modal.panel.SetBorder(true).SetBorderColor(app.Styles.PrimaryTextColor).SetBorderPadding(0, 0, 1, 1)
	modal.panel.
		AddItem(modal.body, 0, 1, true).
		AddItem(modal.status, 1, 0, false)

	modal.AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modal.panel, 0, 6, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	modal.panel.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEscape, event.Key() == tcell.KeyRune && event.Rune() == 'q':
			modal.Close()
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'a':
			modal.toggleAnalyze()
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'y':
			modal.copyPlan()
			return nil
		}
		return event
	})

	modal.updateTitle()
	return modal
}

func (modal *ExplainModal) updateTitle() {
	title := " Query plan "
	if modal.analyzed {
		title = " Query plan (ANALYZE: measured) "
	}
	modal.panel.SetTitle(title)
}

func (modal *ExplainModal) hint() string {
	analyze := "a analyze (runs the query)"
	if modal.analyzed {
		analyze = "a plan only"
	}
	if modal.analyzeBlocked != "" {
		analyze = "analyze unavailable"
	}
	return analyze + " • y copy • q/Esc close"
}

func (modal *ExplainModal) setStatus(message string, isError bool) {
	if isError {
		modal.status.SetText(fmt.Sprintf("[%s]%s[-]", app.Styles.ErrorColor, tview.Escape(message)))
		return
	}
	modal.status.SetText(tview.Escape(message))
}

func (modal *ExplainModal) toggleAnalyze() {
	if modal.loading {
		return
	}
	if !modal.analyzed && modal.analyzeBlocked != "" {
		modal.setStatus(modal.analyzeBlocked, true)
		return
	}
	modal.Run(!modal.analyzed)
}

// Run fetches the plan in the background and renders it when it arrives.
func (modal *ExplainModal) Run(analyze bool) {
	if modal.cancel != nil {
		modal.cancel()
	}
	ctx, cancel := context.WithCancel(app.App.Context())
	modal.cancel = cancel
	modal.generation++
	generation := modal.generation
	modal.loading = true
	modal.setStatus("Explaining… (Esc closes)", false)

	go func() {
		result, err := modal.explain(ctx, analyze)
		App.QueueUpdateDraw(func() {
			if generation != modal.generation {
				return
			}
			modal.loading = false
			if err != nil {
				modal.setStatus(err.Error(), true)
				return
			}
			modal.Render(result)
		})
	}()
}

// Render shows a fetched plan.
func (modal *ExplainModal) Render(result drivers.ExplainResult) {
	modal.analyzed = result.Analyzed
	modal.updateTitle()

	summary := ""
	if result.Format == drivers.ExplainFormatPostgresJSON {
		plan, err := explain.ParsePostgresPlan(result.Text)
		if err != nil {
			// Still show what the database returned.
			modal.showText(result.Text)
			modal.setStatus(err.Error(), true)
			return
		}
		modal.showTree(plan, result.Analyzed)
		if result.Analyzed {
			summary = fmt.Sprintf("Planning %.3f ms • Execution %.3f ms • ", plan.PlanningMs, plan.ExecutionMs)
		}
	} else {
		modal.showText(result.Text)
	}
	modal.setStatus(summary+modal.hint(), false)
}

func (modal *ExplainModal) showTree(plan *explain.Plan, analyzed bool) {
	var build func(node *explain.PlanNode) *tview.TreeNode
	build = func(node *explain.PlanNode) *tview.TreeNode {
		treeNode := tview.NewTreeNode(node.Label(analyzed)).
			SetColor(app.Styles.PrimaryTextColor).
			SetExpanded(true)
		for _, child := range node.Children {
			treeNode.AddChild(build(child))
		}
		return treeNode
	}
	root := build(plan.Root)
	// Enter folds and unfolds a subtree.
	modal.tree.SetSelectedFunc(func(node *tview.TreeNode) {
		node.SetExpanded(!node.IsExpanded())
	})
	modal.tree.SetRoot(root).SetCurrentNode(root)
	modal.planText = plan.Text(analyzed)
	modal.body.SwitchToPage(explainPageTree)
	if modal.panel.HasFocus() {
		App.SetFocus(modal.tree)
	}
}

func (modal *ExplainModal) showText(text string) {
	if strings.TrimSpace(text) == "" {
		text = "The database returned an empty plan."
	}
	modal.planText = text
	modal.text.SetText(text).ScrollToBeginning()
	modal.body.SwitchToPage(explainPageText)
	if modal.panel.HasFocus() {
		App.SetFocus(modal.text)
	}
}

func (modal *ExplainModal) copyPlan() {
	if modal.planText == "" {
		return
	}
	if err := lib.NewClipboard().Write(modal.planText); err != nil {
		logger.Error("Error copying query plan to clipboard", map[string]any{"error": err.Error()})
		modal.setStatus("Could not copy: "+err.Error(), true)
		return
	}
	modal.setStatus("Plan copied • "+modal.hint(), false)
}

// Close cancels a running explain and removes the modal.
func (modal *ExplainModal) Close() {
	if modal.cancel != nil {
		modal.cancel()
	}
	modal.generation++
	if modal.onClose != nil {
		modal.onClose()
	}
}

// showExplainModal opens the Plan Viewer for an editor query.
func showExplainModal(driver drivers.Driver, database, query string) {
	if mainPages == nil {
		return
	}
	previousFocus := App.GetFocus()
	provider := driver.GetProvider()

	explainQuery := func(ctx context.Context, analyze bool) (drivers.ExplainResult, error) {
		return drivers.ExplainQuery(ctx, driver, database, query, analyze)
	}
	modal := NewExplainModal(explainQuery, explainAnalyzeBlockReason(query, provider), func() {
		mainPages.RemovePage(pageNameExplain)
		if previousFocus != nil {
			App.SetFocus(previousFocus)
		}
	})

	mainPages.AddPage(pageNameExplain, modal, true, true)
	App.SetFocus(modal.text)
	// The plan-only form never executes the query, also when the user typed
	// EXPLAIN ANALYZE: measuring is an explicit step inside the viewer.
	modal.Run(false)
}
