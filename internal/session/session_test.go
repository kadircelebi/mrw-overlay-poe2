package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A made-up value in the POESESSID shape; no real session is used in tests.
const fake = "0123456789abcdef0123456789abcdef"

func TestSessionRoundTripIsEncrypted(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if s.Load() != "" || !s.SavedAt().IsZero() {
		t.Fatal("a fresh store must be empty")
	}
	if err := s.Save(fake); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(fake)) {
		t.Fatal("the session is stored in plain text")
	}
	if got := s.Load(); got != fake {
		t.Fatalf("Load() = %q", got)
	}
	if err := s.Clear(); err != nil || s.Load() != "" {
		t.Fatalf("Clear() left %q (%v)", s.Load(), err)
	}
}

func TestSessionRefusesOddValues(t *testing.T) {
	s := New(t.TempDir())
	for _, v := range []string{"", "short", "has space 0123456789abcdef", "0123456789abcdef\r\nX-Evil: 1"} {
		if err := s.Save(v); err == nil {
			t.Errorf("Save(%q) accepted", v)
		}
	}
}

func TestAccountSessionPersistenceAndReplacement(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.SaveAccount(fake, "MrW#1234"); err != nil {
		t.Fatal(err)
	}
	restarted := New(dir)
	if restarted.Load() != fake || restarted.AccountName() != "MrW#1234" {
		t.Fatal("account did not survive restart")
	}
	raw, _ := os.ReadFile(s.path)
	if bytes.Contains(raw, []byte("MrW#1234")) || bytes.Contains(raw, []byte(fake)) {
		t.Fatal("plain account record")
	}
	if err := s.SaveAccount(fake, "Second#5678"); err != nil {
		t.Fatal(err)
	}
	if s.AccountName() != "Second#5678" {
		t.Fatal("old account was retained")
	}
	if err := s.Clear(); err != nil || s.AccountName() != "" {
		t.Fatal("account survived disconnect")
	}
}

func TestLegacyCookieOnlyFileAndInvalidName(t *testing.T) {
	s := New(t.TempDir())
	sealed, err := protect([]byte(fake))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path, sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	if s.Load() != fake || s.AccountName() != "" {
		t.Fatal("legacy file was not loaded")
	}
	for _, name := range []string{"bad\x00name", "bad\nname", strings.Repeat("a", 129)} {
		if s.SaveAccount(fake, name) == nil {
			t.Fatal("invalid name accepted")
		}
	}
	if s.Load() != fake || s.AccountName() != "" {
		t.Fatal("invalid record replaced legacy session")
	}
}
