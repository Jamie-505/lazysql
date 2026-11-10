package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/commands"
	"github.com/jorgerojas26/lazysql/keymap"
)

func TestPaletteGroupsForFocus(t *testing.T) {
	tests := []struct {
		name  string
		focus tview.Primitive
		want  []string
	}{
		{"editor", &SQLEditor{}, []string{app.EditorGroup}},
		{"tree", &Tree{}, []string{app.TreeGroup, app.HomeGroup}},
		{"table", &ResultsTable{}, []string{app.TableGroup, app.HomeGroup}},
		{"connections", &ConnectionsTable{}, []string{app.ConnectionGroup}},
		{"form field", tview.NewInputField(), nil},
		{"nothing", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paletteGroupsForFocus(tt.focus)
			if len(got) != len(tt.want) {
				t.Fatalf("groups = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("groups = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestPaletteActionsListEachCommandOnce(t *testing.T) {
	actions := paletteActions([]string{app.TableGroup, app.HomeGroup})

	seen := map[commands.Command]bool{}
	keys := map[keymap.Key]bool{}
	for _, action := range actions {
		if action.Bind.Cmd == commands.ActionsPalette {
			t.Error("the palette lists itself")
		}
		if seen[action.Bind.Cmd] {
			t.Errorf("command %s is listed twice", action.Bind.Cmd)
		}
		if keys[action.Bind.Key] {
			t.Errorf("key %s is listed twice", action.Bind.Key)
		}
		seen[action.Bind.Cmd] = true
		keys[action.Bind.Key] = true
	}
	for _, want := range []commands.Command{commands.ThemePicker, commands.ShowTableDDL, commands.ServerInfo, commands.ExportCSV} {
		if !seen[want] {
			t.Errorf("command %s is missing", want)
		}
	}
	if seen[commands.ExplainQuery] {
		t.Error("an editor-only command is offered while a table is focused")
	}

	globalOnly := paletteActions(nil)
	if len(globalOnly) == 0 || len(globalOnly) >= len(actions) {
		t.Errorf("global-only palette has %d actions, table palette %d", len(globalOnly), len(actions))
	}
}

func TestFilterPaletteActions(t *testing.T) {
	actions := []paletteAction{
		{Bind: keymap.Bind{Description: "Export to CSV"}},
		{Bind: keymap.Bind{Description: "Explain query (plan viewer)"}},
		{Bind: keymap.Bind{Description: "Sort ascending"}},
	}

	if got := filterPaletteActions(actions, "  "); len(got) != 3 {
		t.Fatalf("blank filter kept %d actions, want all", len(got))
	}
	got := filterPaletteActions(actions, "EXPL")
	if len(got) == 0 || got[0].Bind.Description != "Explain query (plan viewer)" {
		t.Fatalf("filter EXPL = %+v, want the explain action first", got)
	}
	if got := filterPaletteActions(actions, "zzz"); len(got) != 0 {
		t.Fatalf("filter zzz matched %d actions", len(got))
	}
}

func TestKeyEventTriggersItsBinding(t *testing.T) {
	for _, group := range []string{app.TableGroup, app.EditorGroup, app.HomeGroup, app.TreeGroup, app.ConnectionGroup} {
		binds := app.Keymaps.Group(group)
		for _, action := range paletteActions([]string{group}) {
			if action.Group != group {
				continue
			}
			if got := binds.Resolve(keyEvent(action.Bind.Key)); got != action.Bind.Cmd {
				t.Errorf("%s: replaying %s ran %s, want %s", group, action.Bind.Key, got, action.Bind.Cmd)
			}
		}
	}
	if got := app.Keymaps.Resolve(keyEvent(keymap.Key{Code: tcell.KeyCtrlT})); got != commands.ThemePicker {
		t.Errorf("replaying Ctrl-T ran %s", got)
	}
}

func TestActionsPaletteRunsSelectedAction(t *testing.T) {
	actions := []paletteAction{
		{Group: "table", Bind: keymap.Bind{Key: keymap.Key{Char: 'E'}, Cmd: commands.ExportCSV, Description: "Export to CSV"}},
		{Group: "table", Bind: keymap.Bind{Key: keymap.Key{Char: 'D'}, Cmd: commands.ShowTableDDL, Description: "Show table definition (DDL)"}},
	}
	var ran *paletteAction
	closed := false
	palette := NewActionsPalette(actions, func(action paletteAction) { ran = &action }, func() { closed = true })
	capture := palette.filter.GetInputCapture()

	capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)) // wraps to the first
	capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))   // wraps to the last
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if ran == nil || ran.Bind.Cmd != commands.ShowTableDDL {
		t.Fatalf("ran = %+v, want the table definition action", ran)
	}

	palette.fill("csv")
	ran = nil
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if ran == nil || ran.Bind.Cmd != commands.ExportCSV {
		t.Fatalf("after filtering ran = %+v, want export", ran)
	}

	palette.fill("no such action")
	ran = nil
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if ran != nil {
		t.Fatal("Enter on an empty list ran an action")
	}

	capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !closed {
		t.Fatal("Escape did not close the palette")
	}
}
