package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/helpers"
)

func TestBuildConnectionURL(t *testing.T) {
	tests := []struct {
		name string
		spec connectionSpec
		want string
	}{
		{"postgres default ssl", connectionSpec{Provider: drivers.DriverPostgres, Host: "localhost", Port: "5432", User: "app", Password: "pw", Database: "shop"},
			"postgres://app:pw@localhost:5432/shop?sslmode=disable"},
		{"postgres verify-full", connectionSpec{Provider: drivers.DriverPostgres, Host: "db.example.com", Port: "5432", User: "app", Database: "shop", TLS: "verify-full"},
			"postgres://app@db.example.com:5432/shop?sslmode=verify-full"},
		{"password is escaped", connectionSpec{Provider: drivers.DriverPostgres, Host: "h", User: "u", Password: "p@ss/w:rd?", Database: "d"},
			"postgres://u:p%40ss%2Fw%3Ard%3F@h/d?sslmode=disable"},
		{"mysql plain", connectionSpec{Provider: drivers.DriverMySQL, Host: "127.0.0.1", Port: "3306", User: "root", Password: "pw", Database: "shop", TLS: "false"},
			"mysql://root:pw@127.0.0.1:3306/shop"},
		{"mysql tls", connectionSpec{Provider: drivers.DriverMySQL, Host: "h", Port: "3306", User: "root", TLS: "skip-verify"},
			"mysql://root@h:3306/?tls=skip-verify"},
		{"sqlserver encrypted", connectionSpec{Provider: drivers.DriverMSSQL, Host: "h", Port: "1433", User: "sa", Password: "pw", Database: "master", TLS: "true", SkipVerify: true},
			"sqlserver://sa:pw@h:1433?TrustServerCertificate=true&database=master&encrypt=true"},
		{"sqlserver defaults", connectionSpec{Provider: drivers.DriverMSSQL, Host: "h", Port: "1433", User: "sa", TLS: "false"},
			"sqlserver://sa@h:1433"},
		{"clickhouse secure", connectionSpec{Provider: drivers.DriverClickHouse, Host: "h", Port: "9440", User: "default", Database: "logs", TLS: "true", SkipVerify: true},
			"clickhouse://default@h:9440/logs?secure=true&skip_verify=true"},
		{"clickhouse skip verify needs tls", connectionSpec{Provider: drivers.DriverClickHouse, Host: "h", Port: "9000", TLS: "false", SkipVerify: true},
			"clickhouse://h:9000/"},
		{"ipv6 host", connectionSpec{Provider: drivers.DriverPostgres, Host: "::1", Port: "5432", User: "u"},
			"postgres://u@[::1]:5432/?sslmode=disable"},
		{"sqlite", connectionSpec{Provider: drivers.DriverSqlite, File: " /tmp/data.db "}, "sqlite3:/tmp/data.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildConnectionURL(tt.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("url = %q, want %q", got, tt.want)
			}
			// The connection form must accept what the builder produces.
			parsed, err := helpers.ParseConnectionString(got)
			if err != nil {
				t.Fatalf("generated URL does not parse: %v", err)
			}
			if parsed.Driver != tt.spec.Provider {
				t.Fatalf("driver = %q, want %q", parsed.Driver, tt.spec.Provider)
			}
		})
	}
}

func TestBuildConnectionURLErrors(t *testing.T) {
	tests := []struct {
		name string
		spec connectionSpec
		want string
	}{
		{"no host", connectionSpec{Provider: drivers.DriverPostgres}, "host is required"},
		{"bad port", connectionSpec{Provider: drivers.DriverMySQL, Host: "h", Port: "abc"}, "port"},
		{"port out of range", connectionSpec{Provider: drivers.DriverMySQL, Host: "h", Port: "70000"}, "port"},
		{"password without user", connectionSpec{Provider: drivers.DriverMySQL, Host: "h", Password: "pw"}, "user is required"},
		{"no sqlite file", connectionSpec{Provider: drivers.DriverSqlite}, "file is required"},
		{"unknown type", connectionSpec{Provider: "oracle", Host: "h"}, "unknown database type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildConnectionURL(tt.spec)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestEveryConnectionTypeBuildsAValidURLFromItsDefaults(t *testing.T) {
	for _, selected := range connectionTypes {
		if selected.Provider == "" {
			continue
		}
		t.Run(selected.Label, func(t *testing.T) {
			var gotName, gotURL string
			builder := NewConnectionBuilder(func(name, connectionURL string) { gotName, gotURL = name, connectionURL }, func() {})
			builder.SetType(selected)
			builder.Form.GetFormItemByLabel(builderLabelName).(*tview.InputField).SetText(" local ")
			if field, ok := builder.Form.GetFormItemByLabel(builderLabelFile).(*tview.InputField); ok {
				field.SetText("/tmp/data.db")
			}

			for _, mode := range append([]string{""}, selected.TLSModes...) {
				if dropDown, ok := builder.Form.GetFormItemByLabel(selected.TLSLabel).(*tview.DropDown); ok && mode != "" {
					for index, option := range selected.TLSModes {
						if option == mode {
							dropDown.SetCurrentOption(index)
						}
					}
					if got := builder.Spec().TLS; got != mode {
						t.Fatalf("spec TLS = %q, want %q", got, mode)
					}
				}
				gotURL = ""
				builder.finish()
				if gotName != "local" || gotURL == "" {
					t.Fatalf("mode %q: name = %q, url = %q; status %q", mode, gotName, gotURL, builder.StatusText.GetText(true))
				}
				parsed, err := helpers.ParseConnectionString(gotURL)
				if err != nil || parsed.Driver != selected.Provider {
					t.Fatalf("mode %q: url %q parsed to %v, %v", mode, gotURL, parsed, err)
				}
			}
		})
	}
}

func TestConnectionBuilderReportsInvalidDetails(t *testing.T) {
	called := false
	builder := NewConnectionBuilder(func(string, string) { called = true }, func() {})
	builder.SetType(connectionTypes[0])
	builder.Form.GetFormItemByLabel(builderLabelHost).(*tview.InputField).SetText("")

	builder.finish()
	if called {
		t.Fatal("an invalid form continued to the next step")
	}
	if status := builder.StatusText.GetText(true); !strings.Contains(status, "host is required") {
		t.Errorf("status = %q", status)
	}
}

func TestConnectionTypePickerOffersEveryType(t *testing.T) {
	var picked *connectionType
	cancelled := false
	picker := NewConnectionTypePicker(func(selected connectionType) { picked = &selected }, func() { cancelled = true })
	if picker.List.GetItemCount() != len(connectionTypes) {
		t.Fatalf("picker has %d items, want %d", picker.List.GetItemCount(), len(connectionTypes))
	}

	handler := picker.List.InputHandler()
	// j is translated to Down by the input capture; apply it the way tview does.
	handler(picker.List.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)), nil)
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), nil)
	if picked == nil || picked.Provider != drivers.DriverMySQL {
		t.Fatalf("picked = %+v, want MySQL", picked)
	}

	handler(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), nil)
	if !cancelled {
		t.Fatal("Escape did not cancel the picker")
	}
}
