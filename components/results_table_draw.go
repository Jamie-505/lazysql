package components

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
)

var tableBorderRunes = map[rune]bool{
	tview.Borders.Horizontal:  true,
	tview.Borders.Vertical:    true,
	tview.Borders.TopLeft:     true,
	tview.Borders.TopRight:    true,
	tview.Borders.BottomLeft:  true,
	tview.Borders.BottomRight: true,
	tview.Borders.LeftT:       true,
	tview.Borders.RightT:      true,
	tview.Borders.TopT:        true,
	tview.Borders.BottomT:     true,
	tview.Borders.Cross:       true,
}

// Draw draws the table, then takes the selection highlight off the grid
// lines. tview paints the selected cell together with the lines around it, so
// the highlight would stick out over the borders.
func (table *ResultsTable) Draw(screen tcell.Screen) {
	table.Table.Draw(screen)

	x, y, width, height := table.GetInnerRect()
	restoreGridLines(screen, x, y, width, height,
		app.Styles.SecondaryTextColor, table.GetBackgroundColor(), table.gridColor())
}

// gridColor is the color of the grid lines: bright while the table has the
// focus, quiet otherwise.
func (table *ResultsTable) gridColor() tcell.Color {
	if table.state != nil && table.state.gridColor != 0 {
		return table.state.gridColor
	}
	return tview.Styles.GraphicsColor
}

// restoreGridLines gives grid line cells that were painted with the
// selection background their own background and the line color again.
func restoreGridLines(screen tcell.Screen, x, y, width, height int, selectionBg, background, lineColor tcell.Color) {
	for row := y; row < y+height; row++ {
		for column := x; column < x+width; column++ {
			r, combining, style, _ := screen.GetContent(column, row)
			if !tableBorderRunes[r] {
				continue
			}
			if _, bg, _ := style.Decompose(); bg != selectionBg {
				continue
			}
			screen.SetContent(column, row, r, combining, style.Background(background).Foreground(lineColor))
		}
	}
}
