package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/internal/sqlformat"
)

func TestSQLEditorFormatQueryIsOneUndoStep(t *testing.T) {
	editor := NewSQLEditor("")
	editor.SetFormatFunc(func(query string) string {
		return sqlformat.Format(query, sqlDialect(drivers.DriverPostgres))
	})
	const original = "select a from t where b=:name and c = 1"
	editor.SetText(original, true)

	handler := editor.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyCtrlF, 0, tcell.ModCtrl), nil)

	const want = "select a\nfrom t\nwhere b=:name\n  and c = 1"
	if got := editor.GetText(); got != want {
		t.Fatalf("formatted text = %q, want %q", got, want)
	}

	editor.undo()
	if got := editor.GetText(); got != original {
		t.Fatalf("after undo = %q, want the original", got)
	}
}

func TestSQLEditorFormatQueryWithoutFormatterDoesNothing(t *testing.T) {
	editor := NewSQLEditor("")
	editor.SetText("select 1", true)
	editor.InputHandler()(tcell.NewEventKey(tcell.KeyCtrlF, 0, tcell.ModCtrl), nil)
	if got := editor.GetText(); got != "select 1" {
		t.Fatalf("text = %q, want it unchanged", got)
	}
}
