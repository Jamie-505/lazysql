package components

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/internal/secrets"
	"github.com/jorgerojas26/lazysql/models"
)

// secureConnectionPassword moves the password embedded in the connection URL
// into the keyring and leaves a password-free URL to be written to the config
// file. A URL without a password keeps the connection's existing keyring
// entry. The returned warning is non-empty when the keyring could not be used
// and the password therefore stays in the config file.
func secureConnectionPassword(connection *models.Connection) (warning string) {
	if !app.App.Config().StorePasswordsInKeyring {
		return ""
	}
	stripped, password, ok := secrets.SplitPassword(connection.URL)
	if !ok {
		return ""
	}

	account := connection.Keyring
	if account == "" {
		account = uuid.NewString()
	}
	if err := secrets.Default.Set(account, password); err != nil {
		// The inline password must win over any stale keyring entry.
		connection.Keyring = ""
		return fmt.Sprintf("Keyring unavailable (%v): password saved in the config file", err)
	}

	connection.URL = stripped
	connection.Keyring = account
	return ""
}

// resolveConnectionURL returns the URL to hand to a driver: the stored URL
// with the keyring password added. A password written in the URL itself wins.
func resolveConnectionURL(rawURL, account string) (string, error) {
	if account == "" || secrets.HasPassword(rawURL) {
		return rawURL, nil
	}
	password, err := secrets.Default.Get(account)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", errors.New("password not found in the keyring: edit the connection and enter the URL with its password again")
	}
	if err != nil {
		return "", fmt.Errorf("reading password from the keyring: %w", err)
	}
	return secrets.InjectPassword(rawURL, password), nil
}

// forgetConnectionPassword removes the keyring entry of a deleted connection.
func forgetConnectionPassword(connection models.Connection) {
	if connection.Keyring == "" {
		return
	}
	// A missing entry or backend leaves nothing to clean up.
	_ = secrets.Default.Delete(connection.Keyring)
}
