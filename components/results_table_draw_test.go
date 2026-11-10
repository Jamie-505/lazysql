package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestSelectionDoesNotPaintGridLines(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(30, 9)

	selection := tcell.ColorPurple
	background := tcell.ColorBlack
	line := tcell.ColorSilver

	table := tview.NewTable().SetBorders(true).SetSelectable(true, true)
	table.SetBackgroundColor(background)
	table.SetBordersColor(line)
	table.SetSelectedStyle(tcell.StyleDefault.Background(selection).Foreground(tcell.ColorWhite))
	for row := 0; row < 2; row++ {
		for column := 0; column < 2; column++ {
			table.SetCellSimple(row, column, "ab")
		}
	}
	table.SetRect(0, 0, 30, 9)
	table.Select(1, 1)
	table.Draw(screen)

	bg := func(x, y int) tcell.Color {
		_, _, style, _ := screen.GetContent(x, y)
		_, color, _ := style.Decompose()
		return color
	}

	// Cell (1,1) has its text at x=4..5, y=3; its lines are x=3 and x=6, y=2 and y=4.
	if bg(4, 3) != selection {
		t.Fatalf("test setup: selected cell background = %v, want the selection color", bg(4, 3))
	}
	if bg(3, 3) != selection {
		t.Fatalf("test setup: tview no longer paints the line next to the selection; this fix is not needed")
	}

	restoreGridLines(screen, 0, 0, 30, 9, selection, background, line)

	if bg(4, 3) != selection || bg(5, 3) != selection {
		t.Errorf("selected cell lost its highlight")
	}
	for _, position := range [][2]int{{3, 3}, {6, 3}, {4, 2}, {4, 4}, {3, 2}, {6, 4}} {
		r, _, style, _ := screen.GetContent(position[0], position[1])
		fg, color, _ := style.Decompose()
		if color != background || fg != line {
			t.Errorf("grid line %q at %v has bg %v fg %v, want bg %v fg %v", r, position, color, fg, background, line)
		}
	}
}
