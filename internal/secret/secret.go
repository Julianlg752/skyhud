// Package secret keeps the user's DJI API key in the operating system's
// keychain (macOS Keychain, Windows Credential Manager), never in files.
// Nothing here prints, logs or wraps the value into an error.
package secret

import (
	"errors"
	"sync"
)

// ErrNotFound means nothing is stored under the name.
var ErrNotFound = errors.New("secret not found")

// DJIKey is the name the DJI API key is stored under.
const DJIKey = "dji-api-key"

const service = "SkyHUD"

// Store holds named secrets.
type Store interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

// System is the operating system's keychain.
var System Store = system{}

// Memory is an in-memory Store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{m: map[string]string{}} }

func (s *Memory) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *Memory) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = value
	return nil
}

func (s *Memory) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
	return nil
}
