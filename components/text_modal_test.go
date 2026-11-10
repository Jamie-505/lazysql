package components

import (
	"errors"
	"strings"
	"testing"

	"github.com/jorgerojas26/lazysql/drivers"
)

func TestFormatServerInfoAlignsValues(t *testing.T) {
	got := formatServerInfo([]drivers.ServerInfoItem{
		{Label: "Engine", Value: "postgres"},
		{Label: "Database size", Value: "12 MB"},
	})
	want := "Engine         postgres\nDatabase size  12 MB"
	if got != want {
		t.Errorf("formatServerInfo() = %q, want %q", got, want)
	}
}

func TestTextModalShowsContentThenHint(t *testing.T) {
	closed := false
	modal := NewTextModal("Definition of users", func() { closed = true })
	if status := modal.status.GetText(true); !strings.Contains(status, "Loading") {
		t.Errorf("initial status = %q", status)
	}

	modal.SetContent("CREATE TABLE users (id int);")
	if got := modal.text.GetText(true); !strings.Contains(got, "CREATE TABLE users") {
		t.Errorf("text = %q", got)
	}
	if status := modal.status.GetText(true); !strings.Contains(status, "y copy") {
		t.Errorf("status = %q", status)
	}

	modal.SetError(errors.New("permission [denied]"))
	if status := modal.status.GetText(true); !strings.Contains(status, "permission [denied]") {
		t.Errorf("error status = %q, want the full message", status)
	}

	modal.Close()
	if !closed || !modal.closed {
		t.Error("Close did not run the close callback")
	}
}
