package filter

import (
	"encoding/json"
	"os"
	"strings"

	"poe2filter/internal/i18n"
	"poe2filter/internal/prices"
)

// Config contains all filter generation options matching the GUI settings.
type Config struct {
	// Value threshold: items worth less are hidden (or dimmed).
	MinValue     float64 `json:"min_value"`
	MinValueUnit string  `json:"min_value_unit"` // "exalted", "chaos", "divine"

	FilterMode       string `json:"filter_mode"` // "hide", "dim", "show_only"
	IncludeGear      bool   `json:"include_gear"`
	T5Rares          bool   `json:"t5_rares"`
	T5JewelsOnly     bool   `json:"t5_jewels_only"`
	QualityThreshold int    `json:"quality_threshold"` // 0 = off
	DivineTheme      string `json:"divine_theme"`      // mirrors styles["divine"]
	// Styles maps a style group id to a theme id (see StyleGroups).
	Styles map[string]string `json:"styles"`
	// CustomStyles holds the user's own look for groups set to "custom".
	CustomStyles map[string]CustomStyle `json:"custom_styles"`
	DivineSound  string                 `json:"divine_sound,omitempty"` // legacy, migrated into sounds
	// Sounds maps a style group id to a sound choice (see validSound).
	Sounds          map[string]string `json:"sounds"`
	CustomSoundPath string            `json:"custom_sound_path"`
	HideExalt       bool              `json:"hide_exalt"`
	HideGold        bool              `json:"hide_gold"`
	FilterName      string            `json:"filter_name"`
	Whitelist       []string          `json:"whitelist"`
	WhitelistMid    []string          `json:"whitelist_mid"` // shown with a medium highlight
	Blacklist       []string          `json:"blacklist"`
	ChanceBases     []string          `json:"chance_bases"`
	HighWaystones   bool              `json:"high_waystones"`
	HighUncutGems   bool              `json:"high_uncut_gems"`
	// Uncut Support Gems drop constantly, so they have their own switch.
	UncutSupportGems bool   `json:"uncut_support_gems"`
	PinnacleKeys     bool   `json:"boss_keys_and_tablets"`
	LeagueName       string `json:"league_name"`

	// Base filter: a NeverSink strictness (0..6), or a custom file when set.
	Strictness       int    `json:"strictness"`
	CustomBaseFilter string `json:"custom_base_filter"`

	// Exceptional base scanning on the trade API.
	ExceptionalScan bool `json:"exceptional_scan"`
	ScanBudgetPct   int  `json:"scan_budget_pct"` // share of the IP rate limit, 10..80

	// When set, prices come from this collector server URL first.
	PriceSourceURL string `json:"price_source_url"`

	// Language is the interface language: "auto" (follow Windows), "tr", "en"
	// or "zh-Hant".
	Language string `json:"language"`

	AutoUpdateEnabled bool `json:"auto_update_enabled"`
	AutoUpdateHours   int  `json:"auto_update_hours"`
	NotifyEnabled     bool `json:"notify_enabled"`

	// Legacy fields, read once for migration and never written back.
	LegacyMinExalt   float64 `json:"min_exalt,omitempty"`
	LegacyMinDivine  float64 `json:"min_divine,omitempty"`
	LegacyPreset     string  `json:"base_filter_preset,omitempty"`
	LegacyIntervalMn int     `json:"auto_update_interval,omitempty"`
}

// DefaultLeagues is the built-in league list: the fallback for the picker
// when the live list cannot be fetched, and the source of the default league.
var DefaultLeagues = []string{"Forbidden Rites", "HC Forbidden Rites", "Standard", "Hardcore"}

// DefaultConfig returns the recommended out-of-the-box settings.
func DefaultConfig() Config {
	return Config{
		MinValue:          10,
		MinValueUnit:      "exalted",
		FilterMode:        "hide",
		IncludeGear:       true,
		T5Rares:           true,
		T5JewelsOnly:      true,
		DivineTheme:       "neon_cyan",
		FilterName:        "auto_updated",
		Whitelist:         []string{"Mirror of Kalandra", "Albino Rhoa Feather"},
		Blacklist:         []string{},
		ChanceBases:       []string{"Heavy Belt", "Utility Belt"},
		HighWaystones:     true,
		HighUncutGems:     true,
		PinnacleKeys:      true,
		LeagueName:        DefaultLeagues[0],
		Strictness:        3,
		Language:          string(i18n.Auto),
		ExceptionalScan:   true,
		ScanBudgetPct:     40,
		AutoUpdateEnabled: true,
		AutoUpdateHours:   4,
		NotifyEnabled:     true,
	}
}

// LoadConfig reads the config file, migrating old formats. A missing or broken
// file yields the defaults.
func LoadConfig(path string) Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	has := func(k string) bool { _, ok := raw[k]; return ok }

	// v1 used two OR-ed thresholds; the exalt one always won in practice.
	if !has("min_value") && cfg.LegacyMinExalt > 0 {
		cfg.MinValue = cfg.LegacyMinExalt
		cfg.MinValueUnit = "exalted"
	}
	if !has("strictness") {
		switch strings.ToLower(cfg.LegacyPreset) {
		case "structure.filter":
			cfg.Strictness = 0
		case "", "base.filter":
			cfg.Strictness = 6 // the bundled base.filter was UBER-PLUS-STRICT
		default:
			cfg.CustomBaseFilter = cfg.LegacyPreset
		}
	}
	if !has("auto_update_hours") && cfg.LegacyIntervalMn > 0 {
		cfg.AutoUpdateHours = max(1, cfg.LegacyIntervalMn/60)
	}
	cfg.Normalize()
	return cfg
}

// Normalize clamps values into their valid ranges.
func (c *Config) Normalize() {
	c.LegacyMinExalt, c.LegacyMinDivine, c.LegacyPreset, c.LegacyIntervalMn = 0, 0, "", 0
	switch c.MinValueUnit {
	case "exalted", "chaos", "divine":
	default:
		c.MinValueUnit = "exalted"
	}
	if c.MinValue < 0 {
		c.MinValue = 0
	}
	switch c.FilterMode {
	case "hide", "dim", "show_only":
	default:
		c.FilterMode = "hide"
	}
	if c.Strictness < 0 || c.Strictness > 6 {
		c.Strictness = 3
	}
	if c.ScanBudgetPct < 10 || c.ScanBudgetPct > 80 {
		c.ScanBudgetPct = 40
	}
	if !i18n.Valid(c.Language) {
		c.Language = string(i18n.Auto)
	}
	if c.AutoUpdateHours < 1 {
		c.AutoUpdateHours = 4
	}
	c.FilterName = strings.TrimSuffix(strings.TrimSpace(c.FilterName), ".filter")
	if c.FilterName == "" || strings.ContainsAny(c.FilterName, `\/:*?"<>|`) {
		c.FilterName = "auto_updated"
	}
	if c.LeagueName == "" {
		c.LeagueName = DefaultLeagues[0]
	}
	c.normalizeStyles()
	// Empty lists serialise as [] rather than null for the UI.
	for _, l := range []*[]string{&c.Whitelist, &c.WhitelistMid, &c.Blacklist, &c.ChanceBases} {
		if *l == nil {
			*l = []string{}
		}
	}
}

// Save writes the config atomically.
func (c Config) Save(path string) error {
	c.Normalize()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return prices.WriteFileAtomic(path, data)
}

// ThresholdEx converts the configured threshold into Exalted Orbs.
func (c Config) ThresholdEx(r prices.Rates) float64 {
	switch c.MinValueUnit {
	case "divine":
		return c.MinValue * r.DivineEx
	case "chaos":
		if r.ChaosEx > 0 {
			return c.MinValue * r.ChaosEx
		}
	}
	return c.MinValue
}
