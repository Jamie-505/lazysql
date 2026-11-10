package components

import (
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/commands"
	"github.com/jorgerojas26/lazysql/keymap"
)

// paletteAction is one entry of the Actions Palette: a keybinding that can
// be run by name.
type paletteAction struct {
	Group string
	Bind  keymap.Bind
}

// paletteGroupsForFocus returns the keymap groups whose bindings work for the
// focused primitive. Anything else (a dialog, a form field) only gets the
// global actions, because a replayed key would be typed into it.
func paletteGroupsForFocus(focus tview.Primitive) []string {
	switch focus.(type) {
	case *SQLEditor:
		return []string{app.EditorGroup}
	case *Tree:
		return []string{app.TreeGroup, app.HomeGroup}
	case *ResultsTable:
		return []string{app.TableGroup, app.HomeGroup}
	case *ConnectionsTable:
		return []string{app.ConnectionGroup}
	}
	return nil
}

// paletteActions lists the bindings of the groups, then the global ones. A
// command bound to several keys, or reachable through several groups, is
// listed once: under the key that wins when it is pressed.
func paletteActions(groups []string) []paletteAction {
	var actions []paletteAction
	seenCommands := map[commands.Command]bool{commands.ActionsPalette: true}
	seenKeys := map[keymap.Key]bool{}

	add := func(group string, binds keymap.Map) {
		for _, bind := range binds {
			if seenCommands[bind.Cmd] || seenKeys[bind.Key] || bind.Cmd == commands.Noop {
				continue
			}
			seenCommands[bind.Cmd] = true
			seenKeys[bind.Key] = true
			actions = append(actions, paletteAction{Group: group, Bind: bind})
		}
	}

	// Global bindings are resolved first by the application.
	add(app.GlobalGroup, app.Keymaps.Global)
	for _, group := range groups {
		add(group, app.Keymaps.Group(group))
	}
	return actions
}

// filterPaletteActions keeps the actions whose description fuzzy-matches the
// filter, best match first. An empty filter keeps the original order.
func filterPaletteActions(actions []paletteAction, filter string) []paletteAction {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return actions
	}
	type ranked struct {
		action paletteAction
		rank   int
	}
	var matches []ranked
	for _, action := range actions {
		rank := fuzzy.RankMatchNormalizedFold(filter, action.Bind.Description)
		if rank < 0 {
			continue
		}
		matches = append(matches, ranked{action, rank})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].rank < matches[j].rank })

	filtered := make([]paletteAction, len(matches))
	for i, match := range matches {
		filtered[i] = match.action
	}
	return filtered
}

// keyEvent builds the key press that triggers a binding.
func keyEvent(key keymap.Key) *tcell.EventKey {
	if key.Char != 0 {
		return tcell.NewEventKey(tcell.KeyRune, key.Char, tcell.ModNone)
	}
	return tcell.NewEventKey(key.Code, 0, tcell.ModNone)
}

// ActionsPalette is a searchable list of the actions available where the
// user currently is.
type ActionsPalette struct {
	*tview.Flex
	panel   *tview.Flex
	filter  *tview.InputField
	list    *tview.Table
	actions []paletteAction
	shown   []paletteAction
	onRun   func(paletteAction)
	onClose func()
}

func NewActionsPalette(actions []paletteAction, onRun func(paletteAction), onClose func()) *ActionsPalette {
	palette := &ActionsPalette{
		Flex:    tview.NewFlex(),
		panel:   tview.NewFlex().SetDirection(tview.FlexRow),
		filter:  tview.NewInputField(),
		list:    tview.NewTable(),
		actions: actions,
		onRun:   onRun,
		onClose: onClose,
	}

	palette.filter.SetLabel("> ")
	palette.filter.SetFieldBackgroundColor(app.Styles.PrimitiveBackgroundColor)
	palette.filter.SetChangedFunc(palette.fill)

	palette.list.SetSelectable(true, false)
	palette.list.SetSelectedStyle(tcell.StyleDefault.
		Background(app.Styles.SecondaryTextColor).
		Foreground(app.Styles.ContrastSecondaryTextColor))

	// A Flex has a transparent Box; give the panel a solid surface so the
	// page underneath does not show through.
	palette.panel.Box = tview.NewBox()
	palette.panel.SetBorder(true).SetBorderColor(app.Styles.PrimaryTextColor).SetBorderPadding(0, 0, 1, 1)
	palette.panel.SetTitle(" Actions ")
	palette.panel.
		AddItem(palette.filter, 1, 0, true).
		AddItem(palette.list, 0, 1, false)

	palette.AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(palette.panel, 0, 4, true).
			AddItem(nil, 0, 1, false), 70, 0, true).
		AddItem(nil, 0, 1, false)

	// The filter keeps the focus; these keys drive the list from there.
	palette.filter.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			palette.onClose()
			return nil
		case tcell.KeyEnter:
			palette.runSelected()
			return nil
		case tcell.KeyDown, tcell.KeyCtrlN, tcell.KeyTab:
			palette.move(1)
			return nil
		case tcell.KeyUp, tcell.KeyCtrlP, tcell.KeyBacktab:
			palette.move(-1)
			return nil
		}
		return event
	})

	palette.fill("")
	return palette
}

func (palette *ActionsPalette) fill(filter string) {
	palette.shown = filterPaletteActions(palette.actions, filter)
	palette.list.Clear()
	for row, action := range palette.shown {
		palette.list.SetCell(row, 0, tview.NewTableCell(action.Bind.Description).SetExpansion(1))
		palette.list.SetCell(row, 1, tview.NewTableCell(action.Group).
			SetTextColor(app.Styles.TertiaryTextColor).SetAlign(tview.AlignRight))
		palette.list.SetCell(row, 2, tview.NewTableCell(" "+action.Bind.Key.String()).
			SetTextColor(app.Styles.SecondaryTextColor).SetAlign(tview.AlignRight))
	}
	if len(palette.shown) > 0 {
		palette.list.Select(0, 0)
	}
	palette.list.ScrollToBeginning()
}

func (palette *ActionsPalette) move(delta int) {
	if len(palette.shown) == 0 {
		return
	}
	row, _ := palette.list.GetSelection()
	row = (row + delta + len(palette.shown)) % len(palette.shown)
	palette.list.Select(row, 0)
}

func (palette *ActionsPalette) runSelected() {
	row, _ := palette.list.GetSelection()
	if row < 0 || row >= len(palette.shown) {
		return
	}
	palette.onRun(palette.shown[row])
}

// showActionsPalette opens the palette for whatever currently has the focus.
func showActionsPalette() {
	if mainPages == nil || mainPages.HasPage(pageNameActionsPalette) {
		return
	}
	previousFocus := App.GetFocus()
	closePalette := func() {
		mainPages.RemovePage(pageNameActionsPalette)
		if previousFocus != nil {
			App.SetFocus(previousFocus)
		}
	}

	palette := NewActionsPalette(paletteActions(paletteGroupsForFocus(previousFocus)), func(action paletteAction) {
		closePalette()
		// Replay the key so the action runs through the same handler, with
		// the same guards, as when it is pressed.
		App.QueueEvent(keyEvent(action.Bind.Key))
	}, closePalette)

	mainPages.AddPage(pageNameActionsPalette, palette, true, true)
	App.SetFocus(palette.filter)
}
