package secrets

import (
	"net/url"
	"testing"
)

func TestSplitAndInjectPassword(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		stripped string
		password string
		ok       bool
	}{
		{"postgres", "postgres://user:secret@localhost:5432/db?sslmode=disable", "postgres://user@localhost:5432/db?sslmode=disable", "secret", true},
		{"mysql", "mysql://root:pa55@127.0.0.1:3306/shop", "mysql://root@127.0.0.1:3306/shop", "pa55", true},
		{"sqlserver", "sqlserver://sa:Str0ng!@host:1433?database=master", "sqlserver://sa@host:1433?database=master", "Str0ng!", true},
		{"clickhouse", "clickhouse://default:pw@host:9000/default", "clickhouse://default@host:9000/default", "pw", true},
		{"escaped", "postgres://user:p%40ss%2Fw%3Ard@host/db", "postgres://user@host/db", "p@ss/w:rd", true},
		{"raw at sign", "postgres://user:p@ss@host/db", "postgres://user@host/db", "p@ss", true},
		{"password only", "redis://:secret@host", "redis://host", "secret", true},
		{"at sign in query", "postgres://user:secret@host/db?options=a@b", "postgres://user@host/db?options=a@b", "secret", true},
		{"no password", "postgres://user@host/db", "postgres://user@host/db", "", false},
		{"empty password", "postgres://user:@host/db", "postgres://user:@host/db", "", false},
		{"no userinfo", "postgres://host/db", "postgres://host/db", "", false},
		{"sqlite file", "file:data.db", "file:data.db", "", false},
		{"sqlite path", "sqlite3:///tmp/data.db", "sqlite3:///tmp/data.db", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped, password, ok := SplitPassword(tt.url)
			if stripped != tt.stripped || password != tt.password || ok != tt.ok {
				t.Fatalf("SplitPassword(%q) = %q, %q, %v; want %q, %q, %v", tt.url, stripped, password, ok, tt.stripped, tt.password, tt.ok)
			}
			if !ok {
				return
			}

			restored := InjectPassword(stripped, password)
			parsed, err := url.Parse(restored)
			if err != nil {
				t.Fatalf("InjectPassword produced an invalid URL %q: %v", restored, err)
			}
			got, _ := parsed.User.Password()
			if got != tt.password {
				t.Fatalf("round trip password = %q, want %q (url %q)", got, tt.password, restored)
			}
			original, err := url.Parse(tt.stripped)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Host != original.Host || parsed.Path != original.Path || parsed.RawQuery != original.RawQuery || parsed.User.Username() != original.User.Username() {
				t.Fatalf("round trip changed the URL: %q -> %q", tt.stripped, restored)
			}
		})
	}
}

func TestInjectPasswordKeepsExistingPassword(t *testing.T) {
	const withPassword = "postgres://user:inline@host/db"
	if got := InjectPassword(withPassword, "keyring"); got != withPassword {
		t.Fatalf("InjectPassword overwrote an inline password: %q", got)
	}
	if got := InjectPassword("file:data.db", "x"); got != "file:data.db" {
		t.Fatalf("InjectPassword changed an opaque URL: %q", got)
	}
}

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Get("a"); err != ErrNotFound {
		t.Fatalf("Get on empty store: err = %v, want ErrNotFound", err)
	}
	if err := store.Set("a", "secret"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get("a"); err != nil || got != "secret" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := store.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("a"); err != ErrNotFound {
		t.Fatalf("second Delete: err = %v, want ErrNotFound", err)
	}
}
