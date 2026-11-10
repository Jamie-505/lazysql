package components

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/commands"
	"github.com/jorgerojas26/lazysql/helpers/logger"
	"github.com/jorgerojas26/lazysql/lib"
)

// rowPeekList is the column list of a RowPeek. It has its own type so the
// Actions Palette can tell which keys apply while it has focus.
type rowPeekList struct {
	*tview.Table
}

// RowPeek shows every column of one row top to bottom: name, type and value,
// with the full value of the selected column underneath. It is read-only.
type RowPeek struct {
	*tview.Flex
	panel      *tview.Flex
	list       *rowPeekList
	value      *tview.TextView
	status     *tview.TextView
	fields     []rowField
	fullScreen bool
	onClose    func()
}

func rowPeekHint() string {
	key := app.Styles.SecondaryTextColor
	return fmt.Sprintf("[%s]j/k [-]Move  [%s]y [-]Copy value  [%s]Y [-]Copy name and value  [%s]f [-]Full screen  [%s]Esc [-]Close",
		key, key, key, key, key)
}

func NewRowPeek(fields []rowField, onClose func()) *RowPeek {
	peek := &RowPeek{
		Flex:    tview.NewFlex(),
		panel:   tview.NewFlex().SetDirection(tview.FlexRow),
		list:    &rowPeekList{Table: tview.NewTable()},
		value:   tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true),
		status:  tview.NewTextView().SetDynamicColors(true),
		fields:  fields,
		onClose: onClose,
	}

	// A Flex has a transparent Box; give the panel a solid surface so the
	// page underneath does not show through.
	peek.panel.Box = tview.NewBox()
	peek.panel.SetBorder(true).SetBorderColor(app.Styles.PrimaryTextColor).SetBorderPadding(0, 0, 1, 1)
	peek.panel.SetTitle(" Row Details ")

	peek.list.SetSelectable(true, false)
	peek.list.SetFixed(1, 0)
	peek.list.SetSelectedStyle(tcell.StyleDefault.
		Background(app.Styles.SecondaryTextColor).
		Foreground(app.Styles.ContrastSecondaryTextColor))

	peek.value.SetBorder(true).SetBorderColor(app.Styles.InverseTextColor).SetBorderPadding(0, 0, 1, 1)
	peek.status.SetText(rowPeekHint())

	peek.panel.
		AddItem(peek.list, 0, 2, true).
		AddItem(peek.value, 0, 1, false).
		AddItem(peek.status, 1, 0, false)

	peek.fillList()
	peek.list.SetSelectionChangedFunc(func(row, _ int) { peek.showValue(row - 1) })
	peek.list.SetInputCapture(peek.inputCapture)
	peek.list.Select(1, 0)
	peek.showValue(0)
	peek.layout()

	return peek
}

func (peek *RowPeek) fillList() {
	header := tcell.StyleDefault.Foreground(app.Styles.TertiaryTextColor).Background(app.Styles.PrimitiveBackgroundColor)
	for column, title := range []string{"Column", "Type", "Value"} {
		peek.list.SetCell(0, column, tview.NewTableCell(title).SetStyle(header).SetSelectable(false))
	}
	for i, field := range peek.fields {
		value := tview.NewTableCell(tview.Escape(rowPeekSingleLine(field.Value))).SetExpansion(1)
		if field.Pending {
			value.SetBackgroundColor(app.Styles.TableChangeColor)
		}
		peek.list.SetCell(i+1, 0, tview.NewTableCell(tview.Escape(field.Name)).SetTextColor(app.Styles.PrimaryTextColor))
		peek.list.SetCell(i+1, 1, tview.NewTableCell(tview.Escape(field.Type)).SetTextColor(app.Styles.InverseTextColor))
		peek.list.SetCell(i+1, 2, value)
	}
}

// rowPeekSingleLine keeps a value on one list line.
func rowPeekSingleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// rowPeekFullValue is the text of the value pane: JSON objects and arrays
// are indented and highlighted, anything else is shown as it is.
func rowPeekFullValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var indented bytes.Buffer
		if err := json.Indent(&indented, []byte(trimmed), "", "  "); err == nil {
			return colorizeJSON(tview.Escape(indented.String()))
		}
	}
	return tview.Escape(value)
}

func (peek *RowPeek) selectedField() (rowField, bool) {
	row, _ := peek.list.GetSelection()
	if row < 1 || row > len(peek.fields) {
		return rowField{}, false
	}
	return peek.fields[row-1], true
}

func (peek *RowPeek) showValue(index int) {
	if index < 0 || index >= len(peek.fields) {
		peek.value.SetText("")
		return
	}
	field := peek.fields[index]
	peek.value.SetTitle(" " + field.Name + " ")
	peek.value.SetText(rowPeekFullValue(field.Value)).ScrollToBeginning()
}

// layout places the panel centered, or over the whole screen.
func (peek *RowPeek) layout() {
	peek.Clear()
	if peek.fullScreen {
		peek.AddItem(peek.panel, 0, 1, true)
		return
	}
	peek.AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(peek.panel, 0, 6, true).
			AddItem(nil, 0, 1, false), 0, 4, true).
		AddItem(nil, 0, 1, false)
}

// ToggleFullScreen switches between the centered and the full screen layout.
func (peek *RowPeek) ToggleFullScreen() {
	peek.fullScreen = !peek.fullScreen
	peek.layout()
}

// copyText is what a copy puts on the clipboard for the selected column.
func (peek *RowPeek) copyText(withName bool) (string, bool) {
	field, ok := peek.selectedField()
	if !ok {
		return "", false
	}
	if withName {
		return field.Name + ": " + field.Value, true
	}
	return field.Value, true
}

func (peek *RowPeek) copy(withName bool) {
	text, ok := peek.copyText(withName)
	if !ok {
		return
	}
	if err := lib.NewClipboard().Write(text); err != nil {
		logger.Error("Error copying row peek value to clipboard", map[string]any{"error": err.Error()})
		peek.status.SetText(fmt.Sprintf("[%s]%s", app.Styles.ErrorColor, tview.Escape(err.Error())))
		return
	}
	peek.status.SetText("Copied  " + rowPeekHint())
}

func (peek *RowPeek) inputCapture(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		peek.Close()
		return nil
	}

	last := len(peek.fields)
	row, _ := peek.list.GetSelection()

	switch app.Keymaps.Group(app.RowPeekGroup).Resolve(event) {
	case commands.Quit, commands.PeekRow:
		peek.Close()
	case commands.MoveDown:
		peek.list.Select(min(row+1, last), 0)
	case commands.MoveUp:
		peek.list.Select(max(row-1, 1), 0)
	case commands.GotoStart:
		peek.list.Select(1, 0)
	case commands.GotoEnd:
		peek.list.Select(last, 0)
	case commands.Copy:
		peek.copy(false)
	case commands.CopyRowsAs:
		peek.copy(true)
	case commands.ToggleFullScreen:
		peek.ToggleFullScreen()
	default:
		return event
	}
	return nil
}

// Close removes the peek.
func (peek *RowPeek) Close() {
	if peek.onClose != nil {
		peek.onClose()
	}
}

// showRowPeek opens a RowPeek on top of the application.
func showRowPeek(fields []rowField) {
	if mainPages == nil || len(fields) == 0 || mainPages.HasPage(pageNameRowPeek) {
		return
	}
	previousFocus := App.GetFocus()
	peek := NewRowPeek(fields, func() {
		mainPages.RemovePage(pageNameRowPeek)
		if previousFocus != nil {
			App.SetFocus(previousFocus)
		}
	})
	mainPages.AddPage(pageNameRowPeek, peek, true, true)
	App.SetFocus(peek.list)
}
