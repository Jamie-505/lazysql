package components

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/helpers/logger"
	"github.com/jorgerojas26/lazysql/lib"
)

const textModalHint = "y copy • q/Esc close"

// TextModal shows read-only text that is fetched in the background, such as
// a table definition.
type TextModal struct {
	*tview.Flex
	panel  *tview.Flex
	text   *tview.TextView
	status *tview.TextView

	content string
	cancel  context.CancelFunc
	closed  bool
	onClose func()
}

func NewTextModal(title string, onClose func()) *TextModal {
	modal := &TextModal{
		Flex:    tview.NewFlex(),
		panel:   tview.NewFlex().SetDirection(tview.FlexRow),
		text:    tview.NewTextView().SetScrollable(true).SetWrap(false),
		status:  tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter),
		onClose: onClose,
	}

	// A Flex has a transparent Box; give the panel a solid surface so the
	// page underneath does not show through.
	modal.panel.Box = tview.NewBox()
	modal.panel.SetBorder(true).SetBorderColor(app.Styles.PrimaryTextColor).SetBorderPadding(0, 0, 1, 1)
	modal.panel.SetTitle(" " + title + " ")
	modal.panel.
		AddItem(modal.text, 0, 1, true).
		AddItem(modal.status, 1, 0, false)

	modal.AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modal.panel, 0, 6, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	modal.panel.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEscape, event.Key() == tcell.KeyRune && event.Rune() == 'q':
			modal.Close()
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'y':
			modal.copyContent()
			return nil
		}
		return event
	})

	modal.status.SetText("Loading… (Esc closes)")
	return modal
}

// SetContent shows the loaded text.
func (modal *TextModal) SetContent(content string) {
	modal.content = content
	modal.text.SetText(content).ScrollToBeginning()
	modal.status.SetText(textModalHint)
}

// SetError shows why the text could not be loaded.
func (modal *TextModal) SetError(err error) {
	modal.status.SetText(fmt.Sprintf("[%s]%s[-]", app.Styles.ErrorColor, tview.Escape(err.Error())))
}

// Load fetches the text in the background.
func (modal *TextModal) Load(load func(ctx context.Context) (string, error)) {
	ctx, cancel := context.WithCancel(app.App.Context())
	modal.cancel = cancel
	go func() {
		content, err := load(ctx)
		App.QueueUpdateDraw(func() {
			if modal.closed {
				return
			}
			if err != nil {
				modal.SetError(err)
				return
			}
			modal.SetContent(content)
		})
	}()
}

func (modal *TextModal) copyContent() {
	if modal.content == "" {
		return
	}
	if err := lib.NewClipboard().Write(modal.content); err != nil {
		logger.Error("Error copying text to clipboard", map[string]any{"error": err.Error()})
		modal.SetError(err)
		return
	}
	modal.status.SetText("Copied • " + textModalHint)
}

// Close cancels a running load and removes the modal.
func (modal *TextModal) Close() {
	modal.closed = true
	if modal.cancel != nil {
		modal.cancel()
	}
	if modal.onClose != nil {
		modal.onClose()
	}
}

// showTextModal opens a TextModal on top of the application and loads it.
func showTextModal(pageName, title string, load func(ctx context.Context) (string, error)) {
	if mainPages == nil {
		return
	}
	if mainPages.HasPage(pageName) {
		return
	}
	previousFocus := App.GetFocus()
	modal := NewTextModal(title, func() {
		mainPages.RemovePage(pageName)
		if previousFocus != nil {
			App.SetFocus(previousFocus)
		}
	})
	mainPages.AddPage(pageName, modal, true, true)
	App.SetFocus(modal.text)
	modal.Load(load)
}

// showTableDDL opens the definition of a table.
func showTableDDL(driver drivers.Driver, database, table string) {
	showTextModal(pageNameTableDDL, "Definition of "+table, func(ctx context.Context) (string, error) {
		return drivers.TableDDL(ctx, driver, database, table)
	})
}

// formatServerInfo lays the items out as an aligned "label  value" list.
func formatServerInfo(items []drivers.ServerInfoItem) string {
	width := 0
	for _, item := range items {
		if len(item.Label) > width {
			width = len(item.Label)
		}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, item.Label, item.Value))
	}
	return strings.Join(lines, "\n")
}

// showServerInfo opens version and session facts of the connected server.
func showServerInfo(driver drivers.Driver, connectionName string) {
	title := "Server information"
	if connectionName != "" {
		title += ": " + connectionName
	}
	showTextModal(pageNameServerInfo, title, func(ctx context.Context) (string, error) {
		items, err := drivers.ServerInfo(ctx, driver)
		if err != nil {
			return "", err
		}
		return formatServerInfo(items), nil
	})
}
