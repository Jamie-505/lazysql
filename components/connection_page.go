package components

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/models"
)

func NewConnectionPages() *models.ConnectionPages {
	// Create pages component
	pages := tview.NewPages()
	pages.SetBorder(true)

	// Create a grid for both small and large screens
	smallScreenGrid := tview.NewGrid().
		SetRows(0, connectionWindowMinHeight, 0). // Top margin, center height (set while drawing), bottom margin
		SetColumns(1, 0, 1).                      // Left, center, and right columns (center is flexible)
		SetMinSize(1, 1)                          // Minimum cell size

	// Add pages to the center of the small screen grid (with small side margins)
	smallScreenGrid.AddItem(pages, 1, 1, 1, 1, 0, 0, true)

	// Create a grid specifically for large screens with a more compact center box
	largeScreenGrid := tview.NewGrid().
		SetRows(0, connectionWindowMinHeight, 0). // Top margin, center height (set while drawing), bottom margin
		SetColumns(0, 70, 0).                     // Left margin, fixed center width, right margin
		SetMinSize(1, 1)                          // Minimum cell size

	// Add pages to the center of large screen grid
	largeScreenGrid.AddItem(pages, 1, 1, 1, 1, 0, 0, true)

	// Create a responsive grid that switches between small and large layouts
	mainGrid := tview.NewGrid().
		SetRows(0).
		SetColumns(0)

	// Add the small screen layout as default
	mainGrid.AddItem(smallScreenGrid, 0, 0, 1, 1, 0, 0, true)

	// Add the large screen layout for screens with width > 100
	mainGrid.AddItem(largeScreenGrid, 0, 0, 1, 1, 0, 100, true)

	cp := &models.ConnectionPages{
		Grid:  mainGrid,
		Pages: pages,
	}

	connectionForm := NewConnectionForm(cp)
	connectionSelection := NewConnectionSelection(connectionForm, cp)

	cp.AddPage(pageNameConnectionSelection, connectionSelection.Flex, true, true)
	cp.AddPage(pageNameConnectionForm, connectionForm.Flex, true, false)
	builder := addConnectionTypePages(cp, connectionForm)

	// The window grows for pages with more fields than the standard height
	// shows. The height is set while drawing, when the screen size is known.
	fitHeight := func(grid *tview.Grid) {
		grid.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
			contentHeight := 0
			if name, _ := pages.GetFrontPage(); name == pageNameConnectionBuilder {
				contentHeight = builder.Height()
			}
			grid.SetRows(0, connectionWindowHeight(contentHeight, height), 0)
			return x, y, width, height
		})
	}
	fitHeight(smallScreenGrid)
	fitHeight(largeScreenGrid)

	return cp
}
