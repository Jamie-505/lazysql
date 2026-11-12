package components

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/helpers"
	"github.com/jorgerojas26/lazysql/internal/telemetry"
	"github.com/jorgerojas26/lazysql/models"
)

type ConnectionForm struct {
	*tview.Flex
	*tview.Form
	StatusText *tview.TextView
	Action     string
	// backPage is where Esc goes: the details step for a connection being
	// built, the connection list otherwise.
	backPage string
}

func NewConnectionForm(connectionPages *models.ConnectionPages) *ConnectionForm {
	wrapper := tview.NewFlex()

	wrapper.SetDirection(tview.FlexColumnCSS)

	addForm := styleConnectionForm(tview.NewForm())
	addForm.AddInputField("Name", "", 0, nil, nil)
	addForm.AddInputField("URL", "", 0, nil, nil)
	addForm.AddCheckbox("Read-Only", false, nil)
	addForm.AddCheckbox("Show all databases in this instance", false, nil)

	buttonsWrapper := tview.NewFlex().SetDirection(tview.FlexColumn)

	saveButton := tview.NewButton(fmt.Sprintf("[%s]S [-]Save", app.Styles.SecondaryTextColor))
	saveButton.SetStyle(tcell.StyleDefault.Background(app.Styles.PrimitiveBackgroundColor).Foreground(app.Styles.PrimaryTextColor))
	saveButton.SetBorder(true)

	buttonsWrapper.AddItem(saveButton, 0, 1, false)
	buttonsWrapper.AddItem(nil, 1, 0, false)

	testButton := tview.NewButton(fmt.Sprintf("[%s]T [-]Test", app.Styles.SecondaryTextColor))
	testButton.SetStyle(tcell.StyleDefault.Background(app.Styles.PrimitiveBackgroundColor).Foreground(app.Styles.PrimaryTextColor))
	testButton.SetBorder(true)

	buttonsWrapper.AddItem(testButton, 0, 1, false)
	buttonsWrapper.AddItem(nil, 1, 0, false)

	connectButton := tview.NewButton(fmt.Sprintf("[%s]C [-]Connect", app.Styles.SecondaryTextColor))
	connectButton.SetStyle(tcell.StyleDefault.Background(app.Styles.PrimitiveBackgroundColor).Foreground(app.Styles.PrimaryTextColor))
	connectButton.SetBorder(true)

	buttonsWrapper.AddItem(connectButton, 0, 1, false)
	buttonsWrapper.AddItem(nil, 1, 0, false)

	cancelButton := tview.NewButton(fmt.Sprintf("[%s]Esc [-]Back", app.Styles.SecondaryTextColor))
	cancelButton.SetStyle(tcell.StyleDefault.Background(app.Styles.PrimitiveBackgroundColor).Foreground(app.Styles.PrimaryTextColor))
	cancelButton.SetBorder(true)

	buttonsWrapper.AddItem(cancelButton, 0, 1, false)

	statusText := tview.NewTextView()
	statusText.SetBorderPadding(1, 1, 0, 0)

	wrapper.AddItem(addForm, 0, 1, true)
	wrapper.AddItem(statusText, 4, 0, false)
	wrapper.AddItem(buttonsWrapper, 3, 0, false)

	form := &ConnectionForm{
		Flex:       wrapper,
		Form:       addForm,
		StatusText: statusText,
	}

	wrapper.SetInputCapture(form.inputCapture(connectionPages))

	return form
}

func (form *ConnectionForm) inputCapture(connectionPages *models.ConnectionPages) func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			form.goBack(connectionPages)
			return nil
		case tcell.KeyEnter:
			form.save(connectionPages)
			return nil
		}
		switch formHotkey(event) {
		case 's':
			form.save(connectionPages)
			return nil
		case 'c':
			if saved, ok := form.save(connectionPages); ok && connectionSelection != nil {
				go connectionSelection.Connect(saved)
			}
			return nil
		case 't':
			connectionString := form.GetFormItem(1).(*tview.InputField).GetText()
			go form.testConnection(connectionString, form.editedKeyringAccount())
			return nil
		}
		return vimFormNavigation(event)
	}
}

// SetBackPage sets the page Esc returns to.
func (form *ConnectionForm) SetBackPage(page string) {
	form.backPage = page
}

func (form *ConnectionForm) goBack(connectionPages *models.ConnectionPages) {
	page := form.backPage
	if page == "" {
		page = pageNameConnectionSelection
	}
	connectionPages.SwitchToPage(page)
}

// save stores the form as a new or edited connection and returns to the
// connection list. It reports the saved connection.
func (form *ConnectionForm) save(connectionPages *models.ConnectionPages) (models.Connection, bool) {
	connectionName := form.GetFormItem(0).(*tview.InputField).GetText()

	if connectionName == "" {
		form.StatusText.SetText("Connection name is required").SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
		return models.Connection{}, false
	}

	connectionString := form.GetFormItem(1).(*tview.InputField).GetText()

	parsed, err := helpers.ParseConnectionString(connectionString)
	if err != nil {
		form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
		return models.Connection{}, false
	}

	databases := app.App.Connections()
	newDatabases := make([]models.Connection, len(databases))

	readOnly := form.GetFormItem(2).(*tview.Checkbox).IsChecked()
	showAllDatabases := form.GetFormItem(3).(*tview.Checkbox).IsChecked()

	// When the user opts in to browsing every database in the
	// instance, force DBName empty so the tree's InitializeNodes
	// falls back to GetDatabases() instead of pinning to whatever
	// database happens to be embedded in the connection URL.
	var DBName string
	if !showAllDatabases {
		DBName = strings.Split(parsed.Normalize(",", "NULL", 0), ",")[3]

		if DBName == "NULL" {
			DBName = ""
		}
	}

	parsedDatabaseData := models.Connection{
		Name:     connectionName,
		Provider: parsed.Driver,
		DBName:   DBName,
		URL:      connectionString,
		ReadOnly: readOnly,
	}

	var keyringWarning string

	switch form.Action {
	case actionNewConnection:

		keyringWarning = secureConnectionPassword(&parsedDatabaseData)
		newDatabases = append(databases, parsedDatabaseData)
		err := app.App.SaveConnections(newDatabases)
		if err != nil {
			form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
			return models.Connection{}, false
		}

	case actionEditConnection:
		newDatabases = make([]models.Connection, len(databases))
		row, _ := connectionsTable.GetSelection()

		for i, database := range databases {
			if i == row {
				// Start from the existing connection so fields not
				// present on the form (Commands, Username, Password,
				// Hostname, Port, URLParams, Schemas, ...) are preserved.
				updated := database
				// An untouched ${env:...} URL keeps its template in the
				// config file; its credentials already live elsewhere.
				keepsTemplate := database.URL == parsedDatabaseData.URL && app.App.ConnectionUsesTemplate(database.Name)
				updated.Name = parsedDatabaseData.Name
				updated.Provider = parsedDatabaseData.Provider
				updated.DBName = parsedDatabaseData.DBName
				updated.URL = parsedDatabaseData.URL
				updated.ReadOnly = parsedDatabaseData.ReadOnly
				if !keepsTemplate {
					keyringWarning = secureConnectionPassword(&updated)
				}
				newDatabases[i] = updated
			} else {
				newDatabases[i] = database
			}
		}

		err := app.App.SaveConnections(newDatabases)
		if err != nil {
			form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
			return models.Connection{}, false
		}
		if row >= 0 && row < len(newDatabases) {
			parsedDatabaseData = newDatabases[row]
		}
	}

	connectionsTable.SetConnections(newDatabases)
	connectionPages.SwitchToPage(pageNameConnectionSelection)
	if connectionSelection != nil {
		connectionSelection.StatusText.SetText(keyringWarning).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
	}
	return parsedDatabaseData, true
}

// editedKeyringAccount returns the keyring account of the connection being
// edited, so a URL shown without its password can still be tested.
func (form *ConnectionForm) editedKeyringAccount() string {
	if form.Action != actionEditConnection || connectionsTable == nil {
		return ""
	}
	row, _ := connectionsTable.GetSelection()
	connections := connectionsTable.GetConnections()
	if row < 0 || row >= len(connections) {
		return ""
	}
	return connections[row].Keyring
}

func (form *ConnectionForm) testConnection(connectionString, keyringAccount string) {
	parsed, err := helpers.ParseConnectionString(connectionString)
	if err != nil {
		usage.ConnectionFailed(telemetry.InvalidConnection)
		form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
		return
	}

	connectionString, err = resolveConnectionURL(connectionString, keyringAccount)
	if err != nil {
		usage.ConnectionFailed(telemetry.InvalidConnection)
		form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
		App.ForceDraw()
		return
	}

	form.StatusText.SetText("Connecting...").SetTextColor(app.Styles.TertiaryTextColor)

	poolConfig, err := app.App.Config().EffectiveConnectionPool(models.Connection{Provider: parsed.Driver})
	if err != nil {
		form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(tcell.ColorRed))
		return
	}

	var db drivers.Driver

	switch parsed.Driver {
	case drivers.DriverMySQL:
		db = &drivers.MySQL{PoolConfig: poolConfig}
	case drivers.DriverPostgres:
		db = &drivers.Postgres{PoolConfig: poolConfig}
	case drivers.DriverSqlite:
		db = &drivers.SQLite{}
	case drivers.DriverMSSQL:
		db = &drivers.MSSQL{PoolConfig: poolConfig}
	case drivers.DriverClickHouse:
		db = &drivers.ClickHouse{PoolConfig: poolConfig}
	default:
		usage.ConnectionFailed(telemetry.Driver)
		return
	}

	err = db.TestConnection(app.App.Context(), connectionString)

	if err != nil {
		usage.ConnectionFailed(telemetry.ConnectionFailure(err))
		form.StatusText.SetText(err.Error()).SetTextStyle(tcell.StyleDefault.Foreground(app.Styles.ErrorColor).Background(app.Styles.PrimitiveBackgroundColor))
	} else {
		form.StatusText.SetText("Connection success").SetTextColor(app.Styles.TertiaryTextColor)
	}
	App.ForceDraw()
}

func (form *ConnectionForm) SetAction(action string) {
	form.Action = action
}

func (form *ConnectionForm) SetConnectionData(conn models.Connection) {
	form.GetFormItem(0).(*tview.InputField).SetText(conn.Name)
	form.GetFormItem(1).(*tview.InputField).SetText(conn.URL)
	form.GetFormItem(2).(*tview.Checkbox).SetChecked(conn.ReadOnly)
	form.GetFormItem(3).(*tview.Checkbox).SetChecked(showAllDatabasesChecked(conn))
}

// showAllDatabasesChecked infers whether the "show all databases" checkbox
// should be pre-checked when editing an existing connection: true when the
// connection has no DBName pinned but its URL does carry an embedded
// database (i.e. it was previously saved with this option enabled, or the
// user cleared DBName by hand). False for freshly created / legacy
// connections where DBName mirrors the URL, preserving prior behavior.
func showAllDatabasesChecked(conn models.Connection) bool {
	if conn.DBName != "" {
		return false
	}

	parsed, err := helpers.ParseConnectionString(conn.URL)
	if err != nil {
		return false
	}

	urlDBName := strings.Split(parsed.Normalize(",", "NULL", 0), ",")[3]

	return urlDBName != "" && urlDBName != "NULL"
}
