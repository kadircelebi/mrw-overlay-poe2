// Package session keeps the pathofexile.com session the browser extension
// hands over. It is stored encrypted for the current Windows user (DPAPI):
// the file is useless on another machine or to another account, and the
// value is never written anywhere in plain text.
package session

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

// valid is the shape of a POESESSID value; anything else is refused before
// it is stored or sent.
var valid = regexp.MustCompile(`^[A-Za-z0-9]{16,128}$`)

var ErrInvalid = errors.New("session value has an unexpected format")

type Store struct {
	mu   sync.Mutex
	path string
}

func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "data", "pathofexile.session")}
}

func Valid(value string) bool { return valid.MatchString(value) }

func (s *Store) Save(value string) error {
	if !Valid(value) {
		return ErrInvalid
	}
	sealed, err := protect([]byte(value))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return prices.WriteFileAtomic(s.path, sealed)
}

// Load returns the stored session, or "" when there is none (or it can no
// longer be decrypted, e.g. the file came from another machine).
func (s *Store) Load() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	sealed, err := os.ReadFile(s.path)
	if err != nil || len(sealed) == 0 {
		return ""
	}
	plain, err := unprotect(sealed)
	if err != nil || !Valid(string(plain)) {
		return ""
	}
	return string(plain)
}

// SavedAt reports when the session was stored (zero when there is none).
func (s *Store) SavedAt() time.Time {
	fi, err := os.Stat(s.path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
