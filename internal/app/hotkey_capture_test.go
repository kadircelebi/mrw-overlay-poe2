package app

import (
	"poe2filter/internal/overlay"
	"testing"
)

func TestHotkeyCaptureRestoresLatestSettings(t *testing.T) {
	s := &AppService{overlaySettings: overlay.DefaultSettings()}
	var registered overlay.Settings
	s.rebindOverlay = func(_, next overlay.Settings) error { registered = next; return nil }
	if err := s.SetHotkeyCapture(true); err != nil || registered.Enabled {
		t.Fatal("recording did not release shortcuts", err)
	}
	s.overlaySettings.Hotkey = "Ctrl+Alt+K"
	if err := s.SetHotkeyCapture(false); err != nil || registered.Hotkey != "Ctrl+Alt+K" {
		t.Fatal("latest shortcuts not restored", err)
	}
}
