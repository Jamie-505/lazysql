// Package secrets keeps connection passwords out of the config file by
// storing them in the operating system keyring.
package secrets

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// Service is the keyring service name all lazysql entries are stored under.
const Service = "lazysql"

// ErrNotFound reports that the keyring has no entry for the account.
var ErrNotFound = errors.New("no keyring entry found")

// Store reads and writes secrets by account name.
type Store interface {
	Get(account string) (string, error)
	Set(account, secret string) error
	Delete(account string) error
}

// Default is the store used by the application. Tests replace it with a
// MemoryStore.
var Default Store = keyringStore{}

type keyringStore struct{}

func (keyringStore) Get(account string) (string, error) {
	secret, err := keyring.Get(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return secret, err
}

func (keyringStore) Set(account, secret string) error {
	return keyring.Set(Service, account, secret)
}

func (keyringStore) Delete(account string) error {
	err := keyring.Delete(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// MemoryStore is an in-process Store for tests.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]string
	// Err, when set, is returned by every operation to simulate a machine
	// without a keyring backend.
	Err error
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: map[string]string{}}
}

func (s *MemoryStore) Get(account string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return "", s.Err
	}
	secret, ok := s.entries[account]
	if !ok {
		return "", ErrNotFound
	}
	return secret, nil
}

func (s *MemoryStore) Set(account, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	s.entries[account] = secret
	return nil
}

func (s *MemoryStore) Delete(account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	if _, ok := s.entries[account]; !ok {
		return ErrNotFound
	}
	delete(s.entries, account)
	return nil
}

// Len returns the number of stored entries.
func (s *MemoryStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}
