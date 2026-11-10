package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestPaneTitlesShowTheirKeys(t *testing.T) {
	if got := treePaneTitle("docker-postgres"); !strings.Contains(got, "]H[-] docker-postgres ") {
		t.Errorf("tree pane title = %q, want key H before the connection name", got)
	}

	right := rightPaneTitle(false)
	if !strings.Contains(right, "]L[-]") || strings.Contains(right, "Ctrl-E") {
		t.Errorf("right pane title = %q, want only key L", right)
	}
	if strings.Contains(right, "READ-ONLY") {
		t.Errorf("right pane title = %q, want no read-only mark", right)
	}
	if got := rightPaneTitle(true); !strings.Contains(got, "READ-ONLY") {
		t.Errorf("read-only right pane title = %q, want the read-only mark", got)
	}

	if got := paneTitle("", ""); got != "" {
		t.Errorf("title without hints = %q, want empty", got)
	}
}

// focusOnly gives focus to one primitive and takes it from the others.
func focusOnly(target tview.Primitive, all ...tview.Primitive) {
	for _, primitive := range all {
		primitive.Blur()
	}
	target.Focus(func(tview.Primitive) {})
}

func TestHomeInputMode(t *testing.T) {
	tree := &Tree{TreeView: tview.NewTreeView(), Wrapper: tview.NewFlex(), Filter: tview.NewInputField()}
	tree.Wrapper.AddItem(tree.Filter, 1, 0, false).AddItem(tree.TreeView, 0, 1, true)

	changesTable := &ResultsTable{
		Table:  tview.NewTable(),
		Page:   tview.NewPages(),
		state:  &ResultsTableState{},
		Filter: NewResultsFilter(),
	}
	pane := NewTabbedPane()
	pane.AppendTab("users", changesTable, "users")

	home := &Home{Tree: tree, TabbedPane: pane}
	all := []tview.Primitive{tree.TreeView, tree.Filter, changesTable.Table, changesTable.Filter.Input}

	focusOnly(tree.TreeView, all...)
	if got := home.inputMode(); got != inputModeNormal {
		t.Errorf("tree focused: mode = %s, want NORMAL", got)
	}

	focusOnly(tree.Filter, all...)
	if got := home.inputMode(); got != inputModeInsert {
		t.Errorf("tree search focused: mode = %s, want INSERT", got)
	}

	focusOnly(changesTable.Table, all...)
	if got := home.inputMode(); got != inputModeNormal {
		t.Errorf("grid focused: mode = %s, want NORMAL", got)
	}

	changesTable.state.isEditing = true
	if got := home.inputMode(); got != inputModeInsert {
		t.Errorf("cell being edited: mode = %s, want INSERT", got)
	}
	changesTable.state.isEditing = false

	focusOnly(changesTable.Filter.Input, all...)
	if got := home.inputMode(); got != inputModeInsert {
		t.Errorf("WHERE filter focused: mode = %s, want INSERT", got)
	}
}

func TestModeBadgeOnlyOnFocusedPane(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(40, 10)

	tree := &Tree{TreeView: tview.NewTreeView(), Wrapper: tview.NewFlex(), Filter: tview.NewInputField()}
	tree.Wrapper.AddItem(tree.TreeView, 0, 1, true)
	home := &Home{Tree: tree, TabbedPane: NewTabbedPane()}

	read := func() string {
		var text strings.Builder
		for x := 0; x < 12; x++ {
			r, _, _, _ := screen.GetContent(x, 9)
			text.WriteRune(r)
		}
		return text.String()
	}

	home.drawModeBadge(screen, tree.Wrapper, 0, 0, 40, 10)
	if got := read(); strings.Contains(got, "NORMAL") {
		t.Errorf("unfocused pane shows a badge: %q", got)
	}

	tree.TreeView.Focus(func(tview.Primitive) {})
	home.drawModeBadge(screen, tree.Wrapper, 0, 0, 40, 10)
	if got := read(); !strings.Contains(got, " NORMAL ") {
		t.Errorf("focused pane bottom border = %q, want the NORMAL badge", got)
	}
}
