// Package overlay contains the local state and item parsing used by the
// in-game price-check windows. It deliberately keeps UI preferences outside
// filter profiles: changing a farming profile must not change the user's
// global shortcut or window size.
package overlay

import (
	"encoding/json"
	"os"
	"strings"

	"poe2filter/internal/prices"
)

// Settings are stored in overlay.json next to config.json.
type Settings struct {
	Enabled   bool   `json:"enabled"`
	Hotkey    string `json:"hotkey"`
	AutoScale bool   `json:"auto_scale"`
	UIScale   int    `json:"ui_scale"`
}

// DefaultSettings leaves the overlay off: it registers a global shortcut and
// sends keys to the game, so players opt in from Settings.
func DefaultSettings() Settings {
	return Settings{Enabled: false, Hotkey: "Alt+E", AutoScale: true, UIScale: 100}
}

func (s *Settings) Normalize() {
	s.Hotkey = strings.TrimSpace(s.Hotkey)
	if s.Hotkey == "" {
		s.Hotkey = "Alt+E"
	}
	if s.UIScale < 75 {
		s.UIScale = 75
	}
	if s.UIScale > 175 {
		s.UIScale = 175
	}
}

func LoadSettings(path string) Settings {
	s := DefaultSettings()
	raw, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	s.Normalize()
	return s
}

func SaveSettings(path string, s Settings) error {
	s.Normalize()
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return prices.WriteFileAtomic(path, raw)
}
