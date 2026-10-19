package overlay

import (
	"os"
	"path/filepath"
	"slices"
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

// The tracker shortcut comes as Alt+K, unless the player already gave Alt+K
// to something else.
func TestTrackerHotkeyDefault(t *testing.T) {
	s := DefaultSettings()
	s.Normalize()
	if s.TrackerHotkey != "Alt+K" {
		t.Fatalf("default %q", s.TrackerHotkey)
	}
	s.Commands = append(s.Commands, ChatCommand{Hotkey: "Alt+K", Text: "/hideout"})
	s.Normalize()
	if s.TrackerHotkey != "" {
		t.Fatalf("taken Alt+K kept: %q", s.TrackerHotkey)
	}
	s = DefaultSettings()
	s.TrackerHotkey = s.Hotkey
	if s.DistinctHotkeys() == nil {
		t.Fatal("tracker shortcut equal to the price check was accepted")
	}
}

// The inventory scan comes off, with Alt+I ready for when it is turned on;
// a player who already gave Alt+I to something else keeps it, and an
// unknown mode reads as off.
func TestInventoryScanDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"hotkey":"Alt+E"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.InventoryScan != ScanOff || s.InventoryScanHotkey != "Alt+I" {
		t.Fatalf("old file: %q %q", s.InventoryScan, s.InventoryScanHotkey)
	}
	if s := LoadSettings(path); s.LootBoxHidden || s.LootCorner != EarningsCustom || s.LootX != defaultLootX || s.LootY != defaultLootY {
		t.Fatalf("old file's loot box: %v %q %v %v", s.LootBoxHidden, s.LootCorner, s.LootX, s.LootY)
	}
	if s := LoadSettings(path); !slices.Equal(s.LootBoxFields, DefaultLootBoxFields) {
		t.Fatalf("old file's box lines: %v", s.LootBoxFields)
	}
	if err := os.WriteFile(path, []byte(`{"loot_box_fields":["perMap","nonsense","status"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); !slices.Equal(s.LootBoxFields, []string{"status", "perMap"}) {
		t.Fatalf("picked box lines: %v", s.LootBoxFields)
	}
	if err := os.WriteFile(path, []byte(`{"loot_box_fields":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.LootBoxFields == nil || len(s.LootBoxFields) != 0 {
		t.Fatalf("no box lines picked: %v", s.LootBoxFields)
	}
	if err := os.WriteFile(path, []byte(`{"enabled":true,"inventory_scan":"sometimes","chat_commands":[{"hotkey":"Alt+I","text":"/hideout"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(path); s.InventoryScan != ScanOff || s.InventoryScanHotkey != "" || s.DistinctHotkeys() != nil {
		t.Fatalf("taken Alt+I: %q %q %v", s.InventoryScan, s.InventoryScanHotkey, s.DistinctHotkeys())
	}
	s := DefaultSettings()
	s.InventoryScan, s.InventoryScanHotkey = ScanManual, s.MarketHotkey
	if s.DistinctHotkeys() == nil {
		t.Fatal("scan shortcut equal to the market's was accepted")
	}
}
