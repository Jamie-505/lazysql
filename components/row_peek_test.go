package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/models"
)

func TestRowFields(t *testing.T) {
	changes := []models.DBDMLChange{{
		Type:   models.DMLUpdateType,
		Values: []models.CellValue{{Column: "name", TableRowIndex: 2, TableColumnIndex: 1}},
	}}
	table := &ResultsTable{
		Table: tview.NewTable(),
		state: &ResultsTableState{
			columns:         [][]string{{"Field", "Type"}, {"id", "uuid"}, {"name", "text"}},
			listOfDBChanges: &changes,
		},
	}
	for row, values := range [][]string{{"id", "name"}, {"1", "ada"}, {"2", "[grace[]"}} {
		for column, value := range values {
			table.SetCellSimple(row, column, value)
		}
	}

	fields := table.rowFields(2)

	want := []rowField{
		{Name: "id", Type: "uuid", Value: "2"},
		{Name: "name", Type: "text", Value: "[grace]", Pending: true},
	}
	if len(fields) != len(want) {
		t.Fatalf("rowFields returned %d fields, want %d", len(fields), len(want))
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("field %d = %+v, want %+v", i, fields[i], want[i])
		}
	}

	if other := table.rowFields(1); other[1].Pending {
		t.Errorf("row 1 has no pending edit, got %+v", other[1])
	}
}

func testRowPeek(onClose func()) *RowPeek {
	return NewRowPeek([]rowField{
		{Name: "id", Type: "uuid", Value: "c6b1"},
		{Name: "payload", Type: "jsonb", Value: `{"a":[1,2]}`},
		{Name: "note", Type: "text", Value: "line one\nline two"},
	}, onClose)
}

func pressRune(peek *RowPeek, r rune) *tcell.EventKey {
	return peek.inputCapture(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

func TestRowPeekListsEveryColumnOnOneLine(t *testing.T) {
	peek := testRowPeek(nil)

	if rows := peek.list.GetRowCount(); rows != 4 {
		t.Fatalf("list has %d rows, want header + 3 columns", rows)
	}
	if got := peek.list.GetCell(3, 2).Text; got != "line one line two" {
		t.Errorf("multi-line value in list = %q, want it on one line", got)
	}
	if got := peek.list.GetCell(2, 1).Text; got != "jsonb" {
		t.Errorf("type cell = %q, want jsonb", got)
	}
}

func TestRowPeekShowsFullValueOfSelectedColumn(t *testing.T) {
	peek := testRowPeek(nil)

	if got := peek.value.GetText(true); got != "c6b1" {
		t.Fatalf("value pane = %q, want the first column's value", got)
	}

	pressRune(peek, 'j')
	if got := peek.value.GetText(true); !strings.Contains(got, "\"a\": [\n") {
		t.Errorf("JSON value is not indented: %q", got)
	}

	pressRune(peek, 'G')
	if got := peek.value.GetText(true); got != "line one\nline two" {
		t.Errorf("value pane = %q, want the multi-line text unchanged", got)
	}

	pressRune(peek, 'j')
	if row, _ := peek.list.GetSelection(); row != 3 {
		t.Errorf("moving past the last column selected row %d, want 3", row)
	}
	pressRune(peek, 'g')
	pressRune(peek, 'k')
	if row, _ := peek.list.GetSelection(); row != 1 {
		t.Errorf("moving before the first column selected row %d, want 1", row)
	}
}

func TestRowPeekCopyText(t *testing.T) {
	peek := testRowPeek(nil)
	pressRune(peek, 'j')

	if got, _ := peek.copyText(false); got != `{"a":[1,2]}` {
		t.Errorf("copy value = %q, want the raw value", got)
	}
	if got, _ := peek.copyText(true); got != `payload: {"a":[1,2]}` {
		t.Errorf("copy with name = %q", got)
	}
}

func TestRowPeekFullScreenAndClose(t *testing.T) {
	closed := 0
	peek := testRowPeek(func() { closed++ })

	if peek.GetItemCount() != 3 {
		t.Fatalf("centered layout has %d items, want margins around the panel", peek.GetItemCount())
	}
	pressRune(peek, 'f')
	if peek.GetItemCount() != 1 || !peek.fullScreen {
		t.Errorf("full screen layout has %d items, want only the panel", peek.GetItemCount())
	}
	pressRune(peek, 'f')
	if peek.GetItemCount() != 3 {
		t.Errorf("second toggle did not return to the centered layout")
	}

	pressRune(peek, 'q')
	pressRune(peek, 'i')
	peek.inputCapture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if closed != 3 {
		t.Errorf("q, i and Esc closed the peek %d times, want 3", closed)
	}

	if event := pressRune(peek, 'x'); event == nil {
		t.Errorf("an unbound key was swallowed")
	}
}

func TestRowPeekKeyIsFreeInTableGroup(t *testing.T) {
	seen := 0
	for _, bind := range app.Keymaps.Group(app.TableGroup) {
		if bind.Key.Char == 'i' {
			seen++
		}
	}
	for _, group := range []string{app.HomeGroup} {
		for _, bind := range app.Keymaps.Group(group) {
			if bind.Key.Char == 'i' {
				t.Errorf("%s group also binds i (%s); it would win over the row peek", group, bind.Cmd)
			}
		}
	}
	if seen != 1 {
		t.Errorf("table group binds i %d times, want once", seen)
	}
}
