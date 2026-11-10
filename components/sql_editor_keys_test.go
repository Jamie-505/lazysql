package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func runeKey(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
}

func TestSQLEditorChangeToEndOfLine(t *testing.T) {
	editor := NewSQLEditor("")
	editor.SetText("select a, b\nfrom t", true)
	editor.vimMode = VimModeNormal
	editor.cy, editor.cx = 0, 7

	handler := editor.InputHandler()
	handler(runeKey('C'), nil)

	if got := editor.GetText(); got != "select \nfrom t" {
		t.Fatalf("text after C = %q, want the rest of the line deleted", got)
	}
	if editor.vimMode != VimModeInsert {
		t.Fatalf("mode after C = %v, want insert", editor.vimMode)
	}

	handler(runeKey('x'), nil)
	if got := editor.GetText(); got != "select x\nfrom t" {
		t.Fatalf("text after typing = %q, want the new text at the cursor", got)
	}

	editor.vimMode = VimModeNormal
	editor.undo()
	if got := editor.GetText(); got != "select a, b\nfrom t" {
		t.Fatalf("text after undo = %q, want the original line back", got)
	}
}

func TestSQLEditorTabCyclesCompletionsBothWays(t *testing.T) {
	editor := NewSQLEditor("")
	editor.acItems = []CompletionItem{{Text: "select"}, {Text: "set"}, {Text: "show"}}
	editor.acVisible = true
	editor.acSelected = 0
	handler := editor.InputHandler()

	tab := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	backtab := tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)

	steps := []struct {
		name string
		key  *tcell.EventKey
		want int
	}{
		{"shift+tab wraps to the last", backtab, 2},
		{"shift+tab goes back", backtab, 1},
		{"tab goes forward", tab, 2},
		{"tab wraps to the first", tab, 0},
	}
	for _, step := range steps {
		handler(step.key, nil)
		if editor.acSelected != step.want {
			t.Fatalf("%s: selected = %d, want %d", step.name, editor.acSelected, step.want)
		}
		if !editor.acVisible {
			t.Fatalf("%s: the suggestion list closed", step.name)
		}
	}
}
