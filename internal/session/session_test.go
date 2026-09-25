package session

import (
	"bytes"
	"os"
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
