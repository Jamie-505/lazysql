package components

import (
	"errors"
	"strings"
	"testing"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/internal/secrets"
	"github.com/jorgerojas26/lazysql/models"
)

func useMemoryKeyring(t *testing.T) *secrets.MemoryStore {
	t.Helper()
	store := secrets.NewMemoryStore()
	previous := secrets.Default
	secrets.Default = store
	t.Cleanup(func() { secrets.Default = previous })
	return store
}

func TestSecureConnectionPasswordMovesPasswordToKeyring(t *testing.T) {
	store := useMemoryKeyring(t)
	connection := models.Connection{Name: "prod", URL: "postgres://user:s3cret@host:5432/db"}

	if warning := secureConnectionPassword(&connection); warning != "" {
		t.Fatalf("warning = %q, want none", warning)
	}
	if connection.URL != "postgres://user@host:5432/db" {
		t.Errorf("stored URL = %q, want it without the password", connection.URL)
	}
	if connection.Keyring == "" {
		t.Fatal("Keyring account was not set")
	}

	resolved, err := resolveConnectionURL(connection.URL, connection.Keyring)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "postgres://user:s3cret@host:5432/db" {
		t.Errorf("resolved URL = %q", resolved)
	}

	forgetConnectionPassword(connection)
	if store.Len() != 0 {
		t.Errorf("keyring still holds %d entries after delete", store.Len())
	}
}

func TestSecureConnectionPasswordReusesAccountOnEdit(t *testing.T) {
	store := useMemoryKeyring(t)
	connection := models.Connection{Name: "prod", URL: "mysql://root:old@host/db"}
	secureConnectionPassword(&connection)
	account := connection.Keyring

	// Editing without retyping the password keeps the stored one.
	connection.URL = "mysql://root@other-host/db"
	secureConnectionPassword(&connection)
	if connection.Keyring != account {
		t.Fatalf("account changed from %q to %q", account, connection.Keyring)
	}
	if resolved, _ := resolveConnectionURL(connection.URL, connection.Keyring); resolved != "mysql://root:old@other-host/db" {
		t.Errorf("resolved URL = %q, want the old password on the new host", resolved)
	}

	// Typing a new password replaces it under the same account.
	connection.URL = "mysql://root:new@other-host/db"
	secureConnectionPassword(&connection)
	if connection.Keyring != account || store.Len() != 1 {
		t.Fatalf("account = %q, entries = %d; want the single original account", connection.Keyring, store.Len())
	}
	if resolved, _ := resolveConnectionURL(connection.URL, connection.Keyring); resolved != "mysql://root:new@other-host/db" {
		t.Errorf("resolved URL = %q, want the new password", resolved)
	}
}

func TestSecureConnectionPasswordFallsBackToConfigFile(t *testing.T) {
	store := useMemoryKeyring(t)
	store.Err = errors.New("no backend")
	const plain = "postgres://user:s3cret@host/db"
	connection := models.Connection{Name: "prod", URL: plain, Keyring: "stale"}

	warning := secureConnectionPassword(&connection)
	if !strings.Contains(warning, "config file") {
		t.Errorf("warning = %q, want it to say where the password went", warning)
	}
	if connection.URL != plain || connection.Keyring != "" {
		t.Errorf("connection = %+v, want the plain URL and no keyring account", connection)
	}
}

func TestSecureConnectionPasswordCanBeDisabled(t *testing.T) {
	store := useMemoryKeyring(t)
	config := app.App.Config()
	config.StorePasswordsInKeyring = false
	t.Cleanup(func() { config.StorePasswordsInKeyring = true })

	const plain = "postgres://user:s3cret@host/db"
	connection := models.Connection{Name: "prod", URL: plain}
	secureConnectionPassword(&connection)
	if connection.URL != plain || connection.Keyring != "" || store.Len() != 0 {
		t.Errorf("connection = %+v, entries = %d; want it untouched", connection, store.Len())
	}
}

func TestResolveConnectionURL(t *testing.T) {
	store := useMemoryKeyring(t)
	if err := store.Set("acct", "kr"); err != nil {
		t.Fatal(err)
	}

	if got, err := resolveConnectionURL("postgres://user:inline@host/db", "acct"); err != nil || got != "postgres://user:inline@host/db" {
		t.Errorf("inline password: got %q, %v; want it unchanged", got, err)
	}
	if got, err := resolveConnectionURL("file:data.db", ""); err != nil || got != "file:data.db" {
		t.Errorf("no account: got %q, %v; want it unchanged", got, err)
	}
	if _, err := resolveConnectionURL("postgres://user@host/db", "missing"); err == nil || !strings.Contains(err.Error(), "edit the connection") {
		t.Errorf("missing entry: err = %v, want guidance to re-enter the password", err)
	}
}
