package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSQLEditorPasteHandlerInsertsAtCursor(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		cx, cy  int
		pasted  string
		want    string
		wantCX  int
		wantCY  int
	}{
		{
			name:    "single line into empty buffer",
			initial: "",
			pasted:  "SELECT 1",
			want:    "SELECT 1",
			wantCX:  8,
		},
		{
			name:    "single line mid-line",
			initial: "SELECT  FROM t",
			cx:      7,
			pasted:  "id",
			want:    "SELECT id FROM t",
			wantCX:  9,
		},
		{
			name:    "multi-line splits current line",
			initial: "AB",
			cx:      1,
			pasted:  "x\ny\nz",
			want:    "Ax\ny\nzB",
			wantCX:  1,
			wantCY:  2,
		},
		{
			name:    "CRLF is normalized",
			initial: "",
			pasted:  "a\r\nb\rc",
			want:    "a\nb\nc",
			wantCX:  1,
			wantCY:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewSQLEditor("")
			e.SetText(tt.initial, false)
			e.cx, e.cy = tt.cx, tt.cy
			undoBefore := len(e.undoStack)

			e.PasteHandler()(tt.pasted, nil)

			if got := e.GetText(); got != tt.want {
				t.Fatalf("text = %q, want %q", got, tt.want)
			}
			if e.cx != tt.wantCX || e.cy != tt.wantCY {
				t.Fatalf("cursor = (%d,%d), want (%d,%d)", e.cx, e.cy, tt.wantCX, tt.wantCY)
			}
			if len(e.undoStack) != undoBefore+1 {
				t.Fatalf("undo stack grew by %d, want 1", len(e.undoStack)-undoBefore)
			}
		})
	}
}

func TestSQLEditorPasteHandlerEmptyIsNoop(t *testing.T) {
	e := NewSQLEditor("")
	e.SetText("abc", false)
	undoBefore := len(e.undoStack)

	e.PasteHandler()("", nil)

	if e.GetText() != "abc" || len(e.undoStack) != undoBefore {
		t.Fatalf("empty paste changed state")
	}
}

func TestSQLEditorStatusBarShowsOpenTransaction(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 1)

	open := false
	e := NewSQLEditor("")
	e.SetTransactionStatus(func() bool { return open })

	row := func() string {
		screen.Clear()
		e.drawStatusBar(screen, 0, 0, 80, tcell.ColorDefault, tcell.ColorDefault)
		var b strings.Builder
		for x := 0; x < 80; x++ {
			r, _, _, _ := screen.GetContent(x, 0)
			b.WriteRune(r)
		}
		return b.String()
	}

	if strings.Contains(row(), "TRANSACTION OPEN") {
		t.Fatal("indicator shown without a transaction")
	}
	open = true
	if !strings.Contains(row(), "TRANSACTION OPEN") {
		t.Fatal("indicator missing while a transaction is open")
	}
}
