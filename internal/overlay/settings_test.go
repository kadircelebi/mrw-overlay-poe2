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

func TestChatCommandShortcuts(t *testing.T) {
	var old Settings // an overlay.json from before the chat commands
	old.Normalize()
	if got := old.ChatCommands(); len(got) != 4 || got[0] != (ChatCommand{"F5", "/hideout"}) ||
		got[2] != (ChatCommand{"F7", "/invite {last}"}) || got[3].Hotkey != "F8" {
		t.Fatalf("defaults: %v", got)
	}
	// The first build had two fixed shortcuts; a changed one is kept.
	first := Settings{HideoutHotkey: "F9", DndHotkey: "F6"}
	first.Normalize()
	if first.Commands[0] != (ChatCommand{"F9", "/hideout"}) || first.HideoutHotkey != "" {
		t.Fatalf("migration: %+v", first)
	}
	// A list the player emptied stays empty; a half-filled row is not bound.
	emptied := Settings{Commands: []ChatCommand{}}
	emptied.Normalize()
	if len(emptied.Commands) != 0 {
		t.Fatalf("emptied list refilled: %v", emptied.Commands)
	}
	half := Settings{Commands: []ChatCommand{{Hotkey: "F7"}, {Hotkey: "F8", Text: "  @{last}\n sold  "}}}
	half.Normalize()
	if got := half.ChatCommands(); len(got) != 1 || got[0].Text != "@{last} sold" {
		t.Fatalf("ready commands: %v", got)
	}
	if old.PanelHotkey != "F9" {
		t.Fatalf("panel shortcut: %q", old.PanelHotkey)
	}
	s := DefaultSettings()
	if err := s.DistinctHotkeys(); err != nil {
		t.Fatal(err)
	}
	clash := s
	clash.Commands = append([]ChatCommand(nil), s.Commands...)
	clash.Commands[3].Hotkey = "F9" // the panel's key
	if clash.DistinctHotkeys() == nil {
		t.Fatal("a command on the panel's key was accepted")
	}
	s.Commands[1].Hotkey = "alt+e" // same as the price check, in another case
	if s.DistinctHotkeys() == nil {
		t.Fatal("a shortcut used twice was accepted")
	}
}

func TestExpeditionHotkeyDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.json")
	// A file from before the shortcut existed gets Alt+Q…
	if err := os.WriteFile(path, []byte(`{"enabled":true,"hotkey":"Alt+E"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.ExpeditionHotkey != "Alt+Q" {
		t.Fatalf("got %q", s.ExpeditionHotkey)
	}
	// …unless Alt+Q already does something else.
	if err := os.WriteFile(path, []byte(`{"enabled":true,"chat_commands":[{"hotkey":"Alt+Q","text":"/hideout"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.ExpeditionHotkey != "" || s.DistinctHotkeys() != nil {
		t.Fatalf("got %q, %v", s.ExpeditionHotkey, s.DistinctHotkeys())
	}
	// A cleared shortcut stays cleared.
	if err := os.WriteFile(path, []byte(`{"enabled":true,"expedition_hotkey":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.ExpeditionHotkey != "" {
		t.Fatalf("got %q", s.ExpeditionHotkey)
	}
}
