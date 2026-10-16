// Package overlay contains the local state and item parsing used by the
// in-game price-check windows. It deliberately keeps UI preferences outside
// filter profiles: changing a farming profile must not change the user's
// global shortcut or window size.
package overlay

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"

	"poe2filter/internal/prices"
)

// Settings are stored in overlay.json next to config.json.
type Settings struct {
	Enabled bool   `json:"enabled"`
	Hotkey  string `json:"hotkey"`
	// MarketHotkey opens the full market window with a fresh search.
	MarketHotkey string `json:"market_hotkey"`
	CraftHotkey  string `json:"craft_hotkey"`
	// HideHotkey reads the item under the cursor and offers to add it to the
	// loot filter's "hidden by me" list.
	HideHotkey string `json:"hide_hotkey"`
	// ExpeditionHotkey reads Expedition's Runeshape Combinations panel off
	// the screen once and writes each reward's price beside it.
	ExpeditionHotkey string `json:"expedition_hotkey"`
	// ChatEnabled turns on Commands: shortcuts that type a line into the
	// game's chat. Like the overlay it sends keys to the game, so it starts
	// off. The keys are taken only while the game is the active window, so
	// they keep working everywhere else (F5 still reloads a browser page).
	ChatEnabled bool          `json:"chat_enabled"`
	Commands    []ChatCommand `json:"chat_commands"`
	// PanelHotkey opens the main panel, as a click on the tray icon does.
	// Like the commands it is taken only while the game is the active window.
	PanelHotkey string `json:"panel_hotkey"`
	// HideoutHotkey and DndHotkey were the first two fixed commands; an
	// overlay.json that has them gets them as its first two Commands.
	HideoutHotkey string `json:"hideout_hotkey,omitempty"`
	DndHotkey     string `json:"dnd_hotkey,omitempty"`
	AutoScale     bool   `json:"auto_scale"`
	UIScale       int    `json:"ui_scale"`
	// LiveSound is the game alert sound played when a live search finds a
	// listing ("none" = silent); LiveNotify shows a Windows notification.
	LiveSound  string `json:"live_sound"`
	LiveNotify bool   `json:"live_notify"`
	// OffNoticeHidden and ChatNoticeHidden hide the main panel's "price
	// check is off" and "game commands are off" cards for players who keep
	// them off on purpose.
	OffNoticeHidden  bool `json:"off_notice_hidden,omitempty"`
	ChatNoticeHidden bool `json:"chat_notice_hidden,omitempty"`
	// TradeHistory follows the account's merchant sales on the trade site
	// (signed in through the browser link). EarningsOverlay shows their
	// totals over the game, at EarningsCorner. Both start off.
	TradeHistory bool `json:"trade_history,omitempty"`
	// TrackerHotkey opens the tracker window, or closes it when open.
	TrackerHotkey   string `json:"tracker_hotkey"`
	EarningsOverlay bool   `json:"earnings_overlay,omitempty"`
	EarningsCorner  string `json:"earnings_corner,omitempty"`
	// EarningsX and EarningsY place the overlay where the player dragged it
	// (EarningsCorner "custom"): its top-left corner as a share of the
	// game's client area, so it stays put at any resolution.
	EarningsX float64 `json:"earnings_x,omitempty"`
	EarningsY float64 `json:"earnings_y,omitempty"`
}

// Where the earnings overlay sits on the game's screen; "custom" is where
// the player dragged it.
var EarningsCorners = []string{"top-left", "top-center", "top-right", "bottom-left", "bottom-right", EarningsCustom}

const EarningsCustom = "custom"

// DefaultEarningsCorner keeps clear of the minimap (top right) and the
// globes and skill bar (bottom).
const DefaultEarningsCorner = "top-center"

// DefaultSettings leaves the overlay off: it registers a global shortcut and
// sends keys to the game, so players opt in from Settings.
func DefaultSettings() Settings {
	return Settings{Enabled: false, Hotkey: "Alt+E", MarketHotkey: "Alt+M", CraftHotkey: "Alt+F", HideHotkey: "Alt+H", ExpeditionHotkey: "Alt+Q", Commands: DefaultChatCommands(), PanelHotkey: "F9", AutoScale: true, UIScale: 100, LiveSound: DefaultLiveSound, LiveNotify: true, EarningsCorner: DefaultEarningsCorner, TrackerHotkey: "Alt+K"}
}

// DefaultLiveSound is a short chime distinct from the loot filter's drops.
const DefaultLiveSound = "ShExalted"

// ErrSameHotkey is returned when two of the overlay's shortcuts are the same.
var ErrSameHotkey = errors.New("each overlay shortcut must be different")

// uses reports whether another shortcut already takes key.
func (s Settings) uses(key string) bool {
	keys := []string{s.Hotkey, s.MarketHotkey, s.CraftHotkey, s.HideHotkey, s.PanelHotkey, s.HideoutHotkey}
	for _, c := range s.Commands {
		keys = append(keys, c.Hotkey)
	}
	for _, k := range keys {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return true
		}
	}
	return false
}

// DistinctHotkeys reports whether every shortcut differs from the others.
func (s Settings) DistinctHotkeys() error {
	seen := map[string]bool{}
	keys := []string{s.Hotkey, s.MarketHotkey, s.CraftHotkey, s.HideHotkey, s.ExpeditionHotkey, s.TrackerHotkey, s.PanelHotkey}
	for _, c := range s.Commands {
		keys = append(keys, c.Hotkey)
	}
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if seen[k] {
			return ErrSameHotkey
		}
		seen[k] = true
	}
	return nil
}

// ChatCommand is a shortcut and the chat line it sends. In Text, {last}
// stands for the player who whispered last.
type ChatCommand struct {
	Hotkey string `json:"hotkey"`
	Text   string `json:"text"`
}

// MaxChatCommands caps the list; chat lines are short (MaxChatText).
const (
	MaxChatCommands = 10
	MaxChatText     = 200
	LastWhisperer   = "{last}"
)

// DefaultChatCommands: hideout, do not disturb, invite and a ready reply to
// whoever whispered last.
func DefaultChatCommands() []ChatCommand {
	return []ChatCommand{
		{Hotkey: "F5", Text: "/hideout"},
		{Hotkey: "F6", Text: "/dnd"},
		{Hotkey: "F7", Text: "/invite " + LastWhisperer},
		{Hotkey: "F8", Text: "@" + LastWhisperer + " Sold, sorry."},
	}
}

// ChatCommands are the commands ready to bind: a shortcut and a line each.
func (s Settings) ChatCommands() []ChatCommand {
	var out []ChatCommand
	for _, c := range s.Commands {
		if c.Hotkey != "" && c.Text != "" {
			out = append(out, c)
		}
	}
	return out
}

func (s *Settings) Normalize() {
	s.Hotkey = strings.TrimSpace(s.Hotkey)
	if s.Hotkey == "" {
		s.Hotkey = "Alt+E"
	}
	s.MarketHotkey = strings.TrimSpace(s.MarketHotkey)
	if s.MarketHotkey == "" {
		s.MarketHotkey = "Alt+M"
	}
	s.LiveSound = strings.TrimSpace(s.LiveSound)
	s.CraftHotkey = strings.TrimSpace(s.CraftHotkey)
	if s.CraftHotkey == "" {
		s.CraftHotkey = "Alt+F"
	}
	s.HideHotkey = strings.TrimSpace(s.HideHotkey)
	if s.HideHotkey == "" && !s.uses("Alt+H") {
		// A player who already gave Alt+H to something else keeps it; the
		// hide shortcut then stays off until they pick one.
		s.HideHotkey = "Alt+H"
	}
	s.ExpeditionHotkey = strings.TrimSpace(s.ExpeditionHotkey)
	if strings.EqualFold(s.ExpeditionHotkey, "Alt+Q") && s.uses("Alt+Q") {
		// The default arrives with an overlay.json written before it; a
		// player who already gave Alt+Q to something else keeps it.
		s.ExpeditionHotkey = ""
	}
	s.TrackerHotkey = strings.TrimSpace(s.TrackerHotkey)
	if strings.EqualFold(s.TrackerHotkey, "Alt+K") && (s.uses("Alt+K") || strings.EqualFold(s.ExpeditionHotkey, "Alt+K")) {
		// As with Alt+Q: a player who already gave Alt+K to something else
		// keeps it.
		s.TrackerHotkey = ""
	}
	if s.Commands == nil {
		s.Commands = DefaultChatCommands()
		if k := strings.TrimSpace(s.HideoutHotkey); k != "" {
			s.Commands[0].Hotkey = k
		}
		if k := strings.TrimSpace(s.DndHotkey); k != "" {
			s.Commands[1].Hotkey = k
		}
	}
	s.HideoutHotkey, s.DndHotkey = "", ""
	s.PanelHotkey = strings.TrimSpace(s.PanelHotkey)
	if s.PanelHotkey == "" {
		s.PanelHotkey = "F9"
	}
	if len(s.Commands) > MaxChatCommands {
		s.Commands = s.Commands[:MaxChatCommands]
	}
	for i := range s.Commands {
		s.Commands[i].Hotkey = strings.TrimSpace(s.Commands[i].Hotkey)
		// One line: a newline in the text would send the rest as a second
		// message.
		text := strings.Join(strings.Fields(s.Commands[i].Text), " ")
		if r := []rune(text); len(r) > MaxChatText {
			text = string(r[:MaxChatText])
		}
		s.Commands[i].Text = text
	}
	if s.LiveSound == "" {
		s.LiveSound = DefaultLiveSound
	}
	if s.UIScale < 75 {
		s.UIScale = 75
	}
	if s.UIScale > 175 {
		s.UIScale = 175
	}
	if !slices.Contains(EarningsCorners, s.EarningsCorner) {
		s.EarningsCorner = DefaultEarningsCorner
	}
	s.EarningsX = min(max(s.EarningsX, 0), 1)
	s.EarningsY = min(max(s.EarningsY, 0), 1)
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
