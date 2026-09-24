package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlayIsOffUntilEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.json")
	if s := LoadSettings(path); s.Enabled {
		t.Fatal("a fresh install must start with the overlay off")
	}
	// A partial file (e.g. only a custom hotkey) must not switch it on either.
	if err := os.WriteFile(path, []byte(`{"hotkey":"Alt+D"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.Enabled || s.Hotkey != "Alt+D" {
		t.Fatalf("got %+v", s)
	}
	// Someone who switched it on keeps it on.
	if err := SaveSettings(path, Settings{Enabled: true, Hotkey: "Alt+E", UIScale: 100}); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); !s.Enabled {
		t.Fatal("saved opt-in was lost")
	}
}
