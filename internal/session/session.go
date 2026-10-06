// Package session keeps the pathofexile.com session the browser extension
// hands over. It is stored encrypted for the current Windows user (DPAPI):
// the file is useless on another machine or to another account, and the
// value is never written anywhere in plain text.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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
	return s.SaveAccount(value, "")
}

type accountSession struct {
	Session     string `json:"session"`
	AccountName string `json:"accountName,omitempty"`
}

func ValidAccountName(name string) bool {
	return utf8.ValidString(name) && len(name) <= 512 && utf8.RuneCountInString(name) <= 128 &&
		!strings.ContainsFunc(name, unicode.IsControl)
}

// SaveAccount keeps the name and cookie together in the same DPAPI-protected
// record. Linking another account replaces both; old cookie-only files load.
func (s *Store) SaveAccount(value, name string) error {
	if !Valid(value) {
		return ErrInvalid
	}
	name = strings.TrimSpace(name)
	if !ValidAccountName(name) {
		return errors.New("invalid account name")
	}
	raw, err := json.Marshal(accountSession{Session: value, AccountName: name})
	if err != nil {
		return err
	}
	sealed, err := protect(raw)
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
	return s.load().Session
}

func (s *Store) AccountName() string {
	return s.load().AccountName
}

func (s *Store) load() accountSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	sealed, err := os.ReadFile(s.path)
	if err != nil || len(sealed) == 0 {
		return accountSession{}
	}
	plain, err := unprotect(sealed)
	if err != nil {
		return accountSession{}
	}
	if Valid(string(plain)) {
		return accountSession{Session: string(plain)}
	}
	var record accountSession
	if json.Unmarshal(plain, &record) != nil || !Valid(record.Session) || !ValidAccountName(record.AccountName) {
		return accountSession{}
	}
	return record
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
