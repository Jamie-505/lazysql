package components

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/models"
)

// connectionType describes one entry of the database type picker and the
// fields its details form shows.
type connectionType struct {
	Provider    string
	Label       string
	DefaultPort string
	// TLSLabel and TLSModes describe the encryption option. The first mode
	// is the default and means "leave the parameter out" when it is also
	// the driver's default.
	TLSLabel string
	TLSModes []string
	// VerifyLabel is the label of the "do not verify the certificate"
	// checkbox, or empty when the TLS modes already cover it.
	VerifyLabel string
}

// connectionTypes are offered in this order. An empty Provider is the
// "type the URL yourself" entry.
var connectionTypes = []connectionType{
	{Provider: drivers.DriverPostgres, Label: "PostgreSQL", DefaultPort: "5432",
		TLSLabel: "SSL mode", TLSModes: []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}},
	{Provider: drivers.DriverMySQL, Label: "MySQL / MariaDB", DefaultPort: "3306",
		TLSLabel: "TLS", TLSModes: []string{"false", "preferred", "skip-verify", "true"}},
	{Provider: drivers.DriverMSSQL, Label: "SQL Server", DefaultPort: "1433",
		TLSLabel: "Encrypt", TLSModes: []string{"false", "true", "disable"}, VerifyLabel: "Trust server certificate"},
	{Provider: drivers.DriverClickHouse, Label: "ClickHouse", DefaultPort: "9000",
		TLSLabel: "Secure (TLS)", TLSModes: []string{"false", "true"}, VerifyLabel: "Skip certificate verification"},
	{Provider: drivers.DriverSqlite, Label: "SQLite"},
	{Label: "Other (enter a connection URL)"},
}

// connectionSpec is what the user entered in the details form.
type connectionSpec struct {
	Provider string
	Host     string
	Port     string
	User     string
	Password string
	Database string
	// File is the database file, for SQLite.
	File string
	// TLS is one of the type's TLSModes; empty means its default.
	TLS string
	// SkipVerify is the type's VerifyLabel checkbox.
	SkipVerify bool
}

// buildConnectionURL turns the entered details into the connection URL
// lazysql stores and connects with.
func buildConnectionURL(spec connectionSpec) (string, error) {
	if spec.Provider == drivers.DriverSqlite {
		file := strings.TrimSpace(spec.File)
		if file == "" {
			return "", errors.New("database file is required")
		}
		if rest, ok := strings.CutPrefix(file, "~/"); ok {
			if home, err := os.UserHomeDir(); err == nil {
				file = filepath.Join(home, rest)
			}
		}
		return "sqlite3:" + file, nil
	}

	host := strings.TrimSpace(spec.Host)
	if host == "" {
		return "", errors.New("host is required")
	}
	port := strings.TrimSpace(spec.Port)
	if port != "" {
		if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
			return "", fmt.Errorf("port %q is not a number between 1 and 65535", port)
		}
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]" // bare IPv6 address
	}

	result := url.URL{Host: host}
	switch {
	case spec.User != "" && spec.Password != "":
		result.User = url.UserPassword(spec.User, spec.Password)
	case spec.User != "":
		result.User = url.User(spec.User)
	case spec.Password != "":
		return "", errors.New("user is required when a password is set")
	}

	database := strings.TrimSpace(spec.Database)
	query := url.Values{}
	switch spec.Provider {
	case drivers.DriverPostgres:
		result.Scheme = "postgres"
		result.Path = "/" + database
		// lib/pq defaults to "require", so the mode is always written.
		mode := spec.TLS
		if mode == "" {
			mode = "disable"
		}
		query.Set("sslmode", mode)
	case drivers.DriverMySQL:
		result.Scheme = "mysql"
		result.Path = "/" + database
		if spec.TLS != "" && spec.TLS != "false" {
			query.Set("tls", spec.TLS)
		}
	case drivers.DriverMSSQL:
		result.Scheme = "sqlserver"
		if database != "" {
			query.Set("database", database)
		}
		if spec.TLS != "" && spec.TLS != "false" {
			query.Set("encrypt", spec.TLS)
		}
		if spec.SkipVerify {
			query.Set("TrustServerCertificate", "true")
		}
	case drivers.DriverClickHouse:
		result.Scheme = "clickhouse"
		result.Path = "/" + database
		if spec.TLS == "true" {
			query.Set("secure", "true")
			if spec.SkipVerify {
				query.Set("skip_verify", "true")
			}
		}
	default:
		return "", fmt.Errorf("unknown database type %q", spec.Provider)
	}
	result.RawQuery = query.Encode()
	return result.String(), nil
}

// vimListNavigation lets j/k move in a list like the arrow keys.
func vimListNavigation(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
	}
	return event
}

// ConnectionTypePicker is the first step of adding a connection: choosing
// the database type.
type ConnectionTypePicker struct {
	*tview.Flex
	List *tview.List
}

func NewConnectionTypePicker(onPick func(connectionType), onCancel func()) *ConnectionTypePicker {
	picker := &ConnectionTypePicker{
		Flex: tview.NewFlex().SetDirection(tview.FlexRow),
		List: tview.NewList().ShowSecondaryText(false),
	}
	picker.List.SetHighlightFullLine(true)
	picker.List.SetSelectedStyle(tcell.StyleDefault.
		Background(app.Styles.SecondaryTextColor).
		Foreground(app.Styles.ContrastSecondaryTextColor))
	picker.List.SetShortcutColor(app.Styles.SecondaryTextColor)
	for index, connectionType := range connectionTypes {
		picker.List.AddItem(connectionType.Label, "", rune('1'+index), nil)
	}
	picker.List.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		onPick(connectionTypes[index])
	})
	picker.List.SetDoneFunc(onCancel)
	picker.List.SetInputCapture(vimListNavigation)

	title := tview.NewTextView().SetText("Choose a database type")
	title.SetTextColor(app.Styles.TertiaryTextColor)
	hint := tview.NewTextView().SetText("j/k or arrows to move • Enter or number to choose • Esc to cancel")
	hint.SetTextColor(app.Styles.TertiaryTextColor)

	picker.AddItem(title, 2, 0, false).
		AddItem(picker.List, 0, 1, true).
		AddItem(hint, 1, 0, false)
	return picker
}

// Labels of the details form. They double as lookup keys.
const (
	builderLabelName     = "Name"
	builderLabelHost     = "Host"
	builderLabelPort     = "Port"
	builderLabelUser     = "User"
	builderLabelPassword = "Password"
	builderLabelDatabase = "Database"
	builderLabelFile     = "Database file"
)

// ConnectionBuilder is the second step of adding a connection: the details
// of the chosen database type. It produces a connection URL.
type ConnectionBuilder struct {
	*tview.Flex
	Form       *tview.Form
	StatusText *tview.TextView
	header     *tview.TextView
	selected   connectionType
	onDone     func(name, connectionURL string)
	onCancel   func()
}

func NewConnectionBuilder(onDone func(name, connectionURL string), onCancel func()) *ConnectionBuilder {
	builder := &ConnectionBuilder{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		Form:       styleConnectionForm(tview.NewForm()),
		StatusText: tview.NewTextView(),
		header:     tview.NewTextView(),
		onDone:     onDone,
		onCancel:   onCancel,
	}
	builder.StatusText.SetBorderPadding(1, 0, 0, 0)

	key := app.Styles.SecondaryTextColor
	hint := tview.NewTextView().SetDynamicColors(true).
		SetText(fmt.Sprintf("[%s]Tab [-]Move  [%s]F1 [-]Continue  [%s]Esc [-]Back", key, key, key))
	hint.SetTextColor(app.Styles.PrimaryTextColor)

	builder.header.SetTextColor(app.Styles.TertiaryTextColor)

	builder.AddItem(builder.header, 1, 0, false).
		AddItem(builder.Form, 0, 1, true).
		AddItem(builder.StatusText, 3, 0, false).
		AddItem(hint, 1, 0, false)

	builder.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			// Escape inside an open drop-down only closes that list.
			if _, isDropDown := app.App.GetFocus().(*tview.List); isDropDown {
				return event
			}
			builder.onCancel()
			return nil
		case tcell.KeyF1:
			builder.finish()
			return nil
		}
		return event
	})
	return builder
}

// SetType rebuilds the form for a database type, with empty details.
func (builder *ConnectionBuilder) SetType(selected connectionType) {
	builder.selected = selected
	builder.StatusText.SetText("")
	form := builder.Form
	form.Clear(true)
	builder.header.SetText("New " + selected.Label + " connection")

	form.AddInputField(builderLabelName, "", 0, nil, nil)
	if selected.Provider == drivers.DriverSqlite {
		form.AddInputField(builderLabelFile, "", 0, nil, nil)
	} else {
		form.AddInputField(builderLabelHost, "localhost", 0, nil, nil)
		form.AddInputField(builderLabelPort, selected.DefaultPort, 6, nil, nil)
		form.AddInputField(builderLabelUser, "", 0, nil, nil)
		form.AddPasswordField(builderLabelPassword, "", 0, '*', nil)
		form.AddInputField(builderLabelDatabase, "", 0, nil, nil)
		if len(selected.TLSModes) > 0 {
			form.AddDropDown(selected.TLSLabel, selected.TLSModes, 0, nil)
			if dropDown, ok := form.GetFormItemByLabel(selected.TLSLabel).(*tview.DropDown); ok {
				dropDown.SetListStyles(builderFieldStyle(), builderSelectedStyle())
			}
		}
		if selected.VerifyLabel != "" {
			form.AddCheckbox(selected.VerifyLabel, false, nil)
		}
	}
	form.AddButton("Continue", builder.finish)
	form.SetFocus(0)
}

func (builder *ConnectionBuilder) inputText(label string) string {
	if field, ok := builder.Form.GetFormItemByLabel(label).(*tview.InputField); ok {
		return field.GetText()
	}
	return ""
}

// Spec reads the form.
func (builder *ConnectionBuilder) Spec() connectionSpec {
	spec := connectionSpec{
		Provider: builder.selected.Provider,
		Host:     builder.inputText(builderLabelHost),
		Port:     builder.inputText(builderLabelPort),
		User:     builder.inputText(builderLabelUser),
		Password: builder.inputText(builderLabelPassword),
		Database: builder.inputText(builderLabelDatabase),
		File:     builder.inputText(builderLabelFile),
	}
	if dropDown, ok := builder.Form.GetFormItemByLabel(builder.selected.TLSLabel).(*tview.DropDown); ok {
		_, spec.TLS = dropDown.GetCurrentOption()
	}
	if checkbox, ok := builder.Form.GetFormItemByLabel(builder.selected.VerifyLabel).(*tview.Checkbox); ok {
		spec.SkipVerify = checkbox.IsChecked()
	}
	return spec
}

func (builder *ConnectionBuilder) finish() {
	connectionURL, err := buildConnectionURL(builder.Spec())
	if err != nil {
		builder.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.
			Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
		return
	}
	builder.onDone(strings.TrimSpace(builder.inputText(builderLabelName)), connectionURL)
}

// styleConnectionForm gives a connection form the look of the rest of the
// app: fields are a quiet shade of the background and only the focused
// button carries the highlight color.
func styleConnectionForm(form *tview.Form) *tview.Form {
	return form.
		SetLabelColor(app.Styles.PrimaryTextColor).
		SetFieldStyle(builderFieldStyle()).
		SetButtonStyle(builderFieldStyle()).
		SetButtonActivatedStyle(builderSelectedStyle())
}

// builderFieldStyle is the look of a field, an unfocused button and an
// unselected dropdown option.
func builderFieldStyle() tcell.Style {
	return tcell.StyleDefault.
		Background(app.Styles.ContrastBackgroundColor).
		Foreground(app.Styles.PrimaryTextColor)
}

// builderSelectedStyle is the look of the focused button and the selected
// dropdown option. It matches the selected row of the connection list.
func builderSelectedStyle() tcell.Style {
	return tcell.StyleDefault.
		Background(app.Styles.SecondaryTextColor).
		Foreground(app.Styles.ContrastSecondaryTextColor)
}

// Height is the number of rows the builder needs to show every field of the
// selected type without scrolling.
func (builder *ConnectionBuilder) Height() int {
	// A form row is one line plus one line of spacing; then the button row
	// and the form's top and bottom padding.
	formHeight := 2*builder.Form.GetFormItemCount() + 3
	const header, status, hint = 1, 3, 1
	return header + formHeight + status + hint
}

// connectionWindowHeight returns the height of the connection window: enough
// for the content, never less than the standard height, and at most 95% of
// the screen.
func connectionWindowHeight(contentHeight, screenHeight int) int {
	const border = 2
	height := max(contentHeight+border, connectionWindowMinHeight)
	return max(min(height, screenHeight*95/100), 1)
}

// connectionWindowMinHeight is the height the connection list and URL form
// have always used.
const connectionWindowMinHeight = 20

// addConnectionTypePages adds the type picker and details pages in front of
// the connection form, which stays the place where a connection is reviewed,
// tested and saved. It returns the details page.
func addConnectionTypePages(pages *models.ConnectionPages, form *ConnectionForm) *ConnectionBuilder {
	builder := NewConnectionBuilder(func(name, connectionURL string) {
		form.GetFormItemByLabel("Name").(*tview.InputField).SetText(name)
		form.GetFormItemByLabel("URL").(*tview.InputField).SetText(connectionURL)
		form.StatusText.SetText("Review the URL, then F1 to save or F2 to test").SetTextColor(app.Styles.TertiaryTextColor)
		form.Form.SetFocus(0)
		pages.SwitchToPage(pageNameConnectionForm)
	}, func() {
		pages.SwitchToPage(pageNameConnectionType)
	})

	picker := NewConnectionTypePicker(func(selected connectionType) {
		if selected.Provider == "" {
			form.Form.SetFocus(0)
			pages.SwitchToPage(pageNameConnectionForm)
			return
		}
		builder.SetType(selected)
		pages.SwitchToPage(pageNameConnectionBuilder)
	}, func() {
		pages.SwitchToPage(pageNameConnectionSelection)
	})

	pages.AddPage(pageNameConnectionType, picker, true, false)
	pages.AddPage(pageNameConnectionBuilder, builder, true, false)
	return builder
}
