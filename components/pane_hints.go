package components

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/commands"
)

// Input modes shown on the border of the focused pane.
const (
	inputModeNormal = "NORMAL"
	inputModeInsert = "INSERT"
	inputModeVisual = "VISUAL"
)

// keyLabel returns the key bound to a command in a keymap group, or "" when
// the command has no key there.
func keyLabel(group string, command commands.Command) string {
	for _, bind := range app.Keymaps.Group(group) {
		if bind.Cmd == command {
			return strings.Trim(bind.Key.String(), "<>")
		}
	}
	return ""
}

// keyHint formats a key, and the name of what it reaches, for a border title.
func keyHint(key, name string) string {
	if key == "" {
		return tview.Escape(name)
	}
	hint := fmt.Sprintf("[%s]%s[-]", app.Styles.SecondaryTextColor, tview.Escape(key))
	if name != "" {
		hint += " " + tview.Escape(name)
	}
	return hint
}

// paneTitle joins hints into a border title. Empty hints are left out.
func paneTitle(hints ...string) string {
	var shown []string
	for _, hint := range hints {
		if hint != "" {
			shown = append(shown, hint)
		}
	}
	if len(shown) == 0 {
		return ""
	}
	return " " + strings.Join(shown, "  ") + " "
}

// treePaneTitle is the title of the tree pane: its key and the connection.
func treePaneTitle(connectionName string) string {
	return paneTitle(keyHint(keyLabel(app.HomeGroup, commands.MoveLeft), connectionName))
}

// rightPaneTitle is the title of the pane with the tabs: its key, and the
// key that opens the SQL editor in it.
func rightPaneTitle(readOnly bool) string {
	readOnlyHint := ""
	if readOnly {
		readOnlyHint = fmt.Sprintf("[%s]READ-ONLY[-]", app.Styles.ReadOnlyColor)
	}
	return paneTitle(
		keyHint(keyLabel(app.HomeGroup, commands.MoveRight), ""),
		keyHint(keyLabel(app.HomeGroup, commands.SwitchToEditorView), "Editor"),
		readOnlyHint,
	)
}

// inputMode tells whether keys are typed as text right now. In insert mode
// the navigation keys of the home view are not available.
func (home *Home) inputMode() string {
	if home.Tree != nil && home.Tree.Wrapper.HasFocus() {
		if home.Tree.Filter.HasFocus() {
			return inputModeInsert
		}
		return inputModeNormal
	}

	if home.TabbedPane == nil {
		return inputModeNormal
	}
	tab := home.TabbedPane.GetCurrentTab()
	if tab == nil {
		return inputModeNormal
	}
	table, ok := tab.Content.(*ResultsTable)
	if !ok {
		return inputModeNormal
	}

	if table.Editor != nil && table.Editor.HasFocus() {
		switch table.Editor.vimMode {
		case VimModeInsert:
			return inputModeInsert
		case VimModeVisual, VimModeVisualLine:
			return inputModeVisual
		default:
			return inputModeNormal
		}
	}
	if table.Filter != nil && table.Filter.Input.HasFocus() {
		return inputModeInsert
	}
	if table.GetIsEditing() {
		return inputModeInsert
	}
	return inputModeNormal
}

// modeBadgeStyle makes insert mode stand out; normal mode stays quiet.
func modeBadgeStyle(mode string) tcell.Style {
	if mode == inputModeNormal {
		return tcell.StyleDefault.
			Background(app.Styles.PrimitiveBackgroundColor).
			Foreground(app.Styles.PrimaryTextColor)
	}
	return tcell.StyleDefault.
		Background(app.Styles.SecondaryTextColor).
		Foreground(app.Styles.ContrastSecondaryTextColor)
}

// drawModeBadge writes the input mode on the bottom border of a focused
// pane, at the left.
func (home *Home) drawModeBadge(screen tcell.Screen, pane *tview.Flex, x, y, width, height int) {
	if !pane.HasFocus() || height < 2 {
		return
	}
	mode := home.inputMode()
	text := " " + mode + " "
	if width < len(text)+4 {
		return
	}
	style := modeBadgeStyle(mode)
	for i, r := range text {
		screen.SetContent(x+2+i, y+height-1, r, nil, style)
	}
}

// showPaneHints puts the key of each pane on its border and the input mode
// on the border of the focused pane.
func (home *Home) showPaneHints(connectionName string, readOnly bool) {
	home.Tree.Wrapper.SetTitle(treePaneTitle(connectionName))
	home.RightWrapper.SetTitle(rightPaneTitle(readOnly))
	home.RightWrapper.SetTitleColor(app.Styles.PrimaryTextColor)

	tree := home.Tree.Wrapper
	tree.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		home.drawModeBadge(screen, tree, x, y, width, height)
		// Border, plus one column of padding on each side.
		return x + 2, y + 1, width - 4, height - 2
	})
	home.RightWrapper.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		home.drawModeBadge(screen, home.RightWrapper, x, y, width, height)
		return x + 1, y + 1, width - 2, height - 2
	})
}
