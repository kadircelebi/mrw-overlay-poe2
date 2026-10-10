package filter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"poe2filter/internal/i18n"
	"poe2filter/internal/prices"
)

const (
	ItemGroupModeShow  = "show"
	ItemGroupModeHide  = "hide"
	ItemGroupModeValue = "value"
)

// ItemGroup is one of the user's own groups. Show and hide groups act on their
// item list; value groups act on every market-priced item at or above their
// threshold. Every visible group has its own colours and sound.
type ItemGroup struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Items []string `json:"items"`
	// Mode is "show", "hide" or "value". Hide remains in the file so older
	// versions and existing profiles keep their original meaning.
	Mode string `json:"mode,omitempty"`
	Hide bool   `json:"hide"`
	// Always lets a shown group beat the valuable-item styles. Without it a
	// valuable item keeps its stronger highlight, which is what the medium
	// list has always done.
	Always bool `json:"always"`
	// ThresholdValue and ThresholdUnit are used only by value groups.
	ThresholdValue float64 `json:"threshold_value,omitempty"`
	ThresholdUnit  string  `json:"threshold_unit,omitempty"`
}

// StackedBases are the items that drop in stacks (from the base filter's
// StackSize rules). Set for each update, never saved; the automatic stack
// rules are written only for these.
type StackedBases map[string]bool

// MaxMinStack bounds a list entry's stack size ("Verisium|x500"); larger
// stacks do not drop.
const MaxMinStack = 5000

// StyleKey is where this group's colours and sound live in Styles, CustomStyles
// and Sounds.
func (g ItemGroup) StyleKey() string { return UserGroupPrefix + g.ID }

// GroupMode understands both the current Mode field and legacy Hide-only
// configs. Keeping this in one place prevents old profiles changing behaviour.
func (g ItemGroup) GroupMode() string {
	switch g.Mode {
	case ItemGroupModeShow, ItemGroupModeHide, ItemGroupModeValue:
		return g.Mode
	}
	if g.Hide {
		return ItemGroupModeHide
	}
	return ItemGroupModeShow
}

// ThresholdEx converts this value group's threshold into Exalted Orbs.
func (g ItemGroup) ThresholdEx(r prices.Rates) float64 {
	return valueToEx(g.ThresholdValue, g.ThresholdUnit, r)
}

// The two stops before the numbers on a tier or level slider.
const (
	// TierHide hides everything of that kind outright.
	TierHide = -2
	// TierOff writes no rule at all, leaving the decision to the base filter.
	TierOff = -1
)

// configVersion marks the meaning of the stored fields, not the app version.
// Version 2 split the old "off" into TierHide and TierOff.
const configVersion = 4

// LegacyWaystoneDefault was the waystone slider's default while it only
// highlighted the tiers above it. Since version 4 the slider also hides the
// tiers below, and that default would hide T1-T13 for everyone who never
// touched it, so it is read as TierOff (see migrateTierMeaning and the
// public profiles' Apply).
const LegacyWaystoneDefault = 14

// Slider ranges, mirroring what the game can produce.
const (
	MaxRareTier        = 5  // UnidentifiedItemTier
	MaxUncutGemLevel   = 20 // GemLevel of skill and spirit gems
	MaxSupportGemLevel = 5  // GemLevel of support gems
	MaxWaystoneTier    = 15 // WaystoneTier
)

// MaxItemGroups keeps the settings panel (and the filter) manageable.
const MaxItemGroups = 12

// Config contains all filter generation options matching the GUI settings.
type Config struct {
	// Stacked is filled by the engine before each update (see StackedBases).
	Stacked StackedBases `json:"-"`
	// Exotics are NeverSink's exotic gear rules, verbatim (neversink.ExoticBlocks),
	// filled by the engine like Stacked.
	Exotics []string `json:"-"`
	// ShowExotics repeats those rules ahead of ours, so the leftover-gear hide
	// cannot swallow valuable bases (Absent/Lament/Portent Amulet...) or
	// identified items with a valuable modifier.
	ShowExotics bool `json:"show_exotics"`
	// HiddenItems is the "hidden by me" list (Alt+H); HiddenOff switches it
	// off without losing it.
	HiddenItems []HiddenItem `json:"hidden_items"`
	// Exotic is the player's changes to the Exotic group (see exotic.go).
	Exotic    ExoticSettings `json:"exotic"`
	HiddenOff bool           `json:"hidden_off"`

	// Value threshold: items worth less are hidden (or dimmed).
	MinValue     float64 `json:"min_value"`
	MinValueUnit string  `json:"min_value_unit"` // "exalted", "chaos", "divine"

	FilterMode  string `json:"filter_mode"` // "hide", "dim", "show_only"
	IncludeGear bool   `json:"include_gear"`
	// T5RareTier shows unidentified rare equipment from this tier up, and
	// RareJewelTier does the same for rare jewels. TierOff switches them off.
	T5RareTier       int    `json:"t5_rare_tier"`
	RareJewelTier    int    `json:"rare_jewel_tier"`
	QualityThreshold int    `json:"quality_threshold"` // 0 = off
	DivineTheme      string `json:"divine_theme"`      // mirrors styles["divine"]
	// Styles maps a style group id to a theme id (see StyleGroups).
	Styles map[string]string `json:"styles"`
	// CustomStyles holds the user's own look for groups set to "custom".
	CustomStyles map[string]CustomStyle `json:"custom_styles"`
	DivineSound  string                 `json:"divine_sound,omitempty"` // legacy, migrated into sounds
	// Sounds maps a style group id to a sound choice (see validSound).
	Sounds map[string]string `json:"sounds"`
	// FontSizes maps a style group id to the label size its rules write
	// (MinFontSize..MaxFontSize); a missing group keeps its built-in size.
	FontSizes map[string]int `json:"font_sizes"`
	// Volumes maps a style group id to the volume its sound plays at
	// (MinSoundVolume..MaxSoundVolume-1); a missing group plays at the maximum.
	Volumes    map[string]int `json:"volumes"`
	HideExalt  bool           `json:"hide_exalt"`
	HideGold   bool           `json:"hide_gold"`
	FilterName string         `json:"filter_name"`
	Whitelist  []string       `json:"whitelist"`
	// ItemGroups are the user's own lists. Each one shows or hides its items
	// and carries its own colours and sound, keyed by ItemGroup.StyleKey().
	ItemGroups  []ItemGroup `json:"item_groups"`
	ChanceBases []string    `json:"chance_bases"`
	// WaystoneTier shows waystones from this tier up and hides the lower ones
	// (1..15, TierOff = NeverSink decides), and UncutGemLevel does the same for
	// uncut skill and spirit gems. A drop whose price reaches a value group is
	// still shown by that group, whatever the slider says.
	WaystoneTier  int `json:"waystone_tier"`
	UncutGemLevel int `json:"uncut_gem_level"`
	// Uncut Support Gems drop constantly, so they have their own switch.
	// UncutSupportLevel is the same for support gems, which drop far more often;
	// TierOff hides them all.
	UncutSupportLevel int    `json:"uncut_support_level"`
	PinnacleKeys      bool   `json:"boss_keys_and_tablets"`
	LeagueName        string `json:"league_name"`
	// LeagueAuto lets the app move LeagueName to the current league
	// (collector.AutoLeague) whenever the trade league list changes.
	LeagueAuto bool `json:"league_auto"`

	// Base filter: a NeverSink strictness (0..6), or a custom file when set.
	Strictness       int    `json:"strictness"`
	CustomBaseFilter string `json:"custom_base_filter"`

	// Exceptional base scanning on the trade API.
	ExceptionalScan bool `json:"exceptional_scan"`
	// SharedScan uses the exceptional prices the scan servers publish. When
	// they cover the league, the player's own scanner stays idle (and keeps
	// their trade quota free); otherwise ExceptionalScan decides.
	SharedScan    bool `json:"shared_scan"`
	ScanBudgetPct int  `json:"scan_budget_pct"` // share of the IP rate limit, 10..80

	// ConfigVersion is the format of this file, used to migrate meanings that
	// changed without the field itself changing.
	ConfigVersion int `json:"config_version"`

	// Language is the interface language: "auto" (follow Windows), "tr", "en"
	// or "zh-Hant".
	Language string `json:"language"`

	AutoUpdateEnabled bool `json:"auto_update_enabled"`
	AutoUpdateHours   int  `json:"auto_update_hours"`
	NotifyEnabled     bool `json:"notify_enabled"`

	// Legacy fields, read once for migration and never written back.
	LegacyT5Rares          *bool    `json:"t5_rares,omitempty"`
	LegacyT5JewelsOnly     *bool    `json:"t5_jewels_only,omitempty"`
	LegacyHighWaystones    *bool    `json:"high_waystones,omitempty"`
	LegacyHighUncutGems    *bool    `json:"high_uncut_gems,omitempty"`
	LegacyUncutSupportGems *bool    `json:"uncut_support_gems,omitempty"`
	LegacyWhitelistMid     []string `json:"whitelist_mid,omitempty"`
	LegacyBlacklist        []string `json:"blacklist,omitempty"`
	LegacyMinExalt         float64  `json:"min_exalt,omitempty"`
	LegacyMinDivine        float64  `json:"min_divine,omitempty"`
	LegacyPreset           string   `json:"base_filter_preset,omitempty"`
	LegacyIntervalMn       int      `json:"auto_update_interval,omitempty"`
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
		T5RareTier:        MaxRareTier,
		RareJewelTier:     MaxRareTier,
		DivineTheme:       "neon_cyan",
		FilterName:        "auto_updated",
		Whitelist:         []string{"Mirror of Kalandra", "Albino Rhoa Feather"},
		ChanceBases:       []string{"Heavy Belt", "Utility Belt"},
		WaystoneTier:      TierOff,
		ShowExotics:       true,
		UncutGemLevel:     MaxUncutGemLevel,
		UncutSupportLevel: TierHide,
		PinnacleKeys:      true,
		LeagueName:        DefaultLeagues[0],
		LeagueAuto:        true,
		Strictness:        3,
		ConfigVersion:     configVersion,
		Language:          string(i18n.Auto),
		ExceptionalScan:   true,
		SharedScan:        true,
		ScanBudgetPct:     40,
		AutoUpdateEnabled: true,
		AutoUpdateHours:   4,
		NotifyEnabled:     true,
	}
}

// legacyDefaultLeague is the built-in league of the versions before
// league_auto existed.
const legacyDefaultLeague = "Forbidden Rites"

// UnmarshalJSON reads a config and, for one written before league_auto
// existed (config.json, saved and exported profiles), sets it: such a file
// held the built-in default league as if picked by hand, so a config still on
// it follows the current league from now on, while any other league was a
// real choice and stays.
func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config // no recursion
	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	if _, ok := raw["league_auto"]; !ok {
		name := strings.TrimSpace(c.LeagueName)
		c.LeagueAuto = name == "" || strings.EqualFold(name, legacyDefaultLeague)
	}
	return nil
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
	// Only the file may claim a format version; the defaults must not, or a
	// file written before the field existed would look current.
	cfg.ConfigVersion = 0
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
	c.HiddenItems = normalizeHidden(c.HiddenItems)
	normalizeExotic(&c.Exotic)
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
	c.migrateTiers()
	c.migrateTierMeaning()
	c.clampTiers()
	c.migrateLists()
	c.normalizeGroups()
	for _, l := range []*[]string{&c.Whitelist, &c.ChanceBases} {
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

// FilterKey identifies the settings the written filter depends on. Two
// configs with the same key write the same rules; the app-only settings
// (language, schedule, notifications, scan budget and source) are left out,
// matching what the panel treats as not needing a rewrite.
func (c Config) FilterKey() string {
	c.Language, c.AutoUpdateEnabled, c.AutoUpdateHours, c.NotifyEnabled = "", false, 0, false
	c.ScanBudgetPct, c.ExceptionalScan, c.LeagueAuto = 0, false, false
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	// An empty list or map and a missing one write the same filter.
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	for k, v := range fields {
		switch v := v.(type) {
		case nil:
			delete(fields, k)
		case map[string]any:
			if len(v) == 0 {
				delete(fields, k)
			}
		case []any:
			if len(v) == 0 {
				delete(fields, k)
			}
		}
	}
	b, _ := json.Marshal(fields) // map keys come out sorted
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ThresholdEx converts the configured threshold into Exalted Orbs.
func (c Config) ThresholdEx(r prices.Rates) float64 {
	return valueToEx(c.MinValue, c.MinValueUnit, r)
}

func valueToEx(value float64, unit string, r prices.Rates) float64 {
	switch unit {
	case "divine":
		return value * r.DivineEx
	case "chaos":
		if r.ChaosEx > 0 {
			return value * r.ChaosEx
		}
	}
	return value
}

// migrateLists folds the two fixed lists of earlier versions into user groups,
// carrying their colours and sound over, so nobody loses a setting on upgrade.
func (c *Config) migrateLists() {
	move := func(items []string, from string, hide bool, name string) {
		if len(items) == 0 {
			return
		}
		g := ItemGroup{ID: c.freeGroupID(), Name: name, Items: items, Hide: hide}
		if v, ok := c.Styles[from]; ok {
			if c.Styles == nil {
				c.Styles = map[string]string{}
			}
			c.Styles[g.StyleKey()] = v
			delete(c.Styles, from)
		}
		if v, ok := c.CustomStyles[from]; ok {
			c.CustomStyles[g.StyleKey()] = v
			delete(c.CustomStyles, from)
		}
		if v, ok := c.Sounds[from]; ok {
			c.Sounds[g.StyleKey()] = v
			delete(c.Sounds, from)
		}
		c.ItemGroups = append(c.ItemGroups, g)
	}
	move(c.LegacyWhitelistMid, GroupWhitelistMid, false, i18n.T("group.migratedMid"))
	move(c.LegacyBlacklist, "", true, i18n.T("group.migratedHide"))
	c.LegacyWhitelistMid, c.LegacyBlacklist = nil, nil
}

// CleanText trims s and turns control characters (line breaks included) into
// spaces, so text the player typed or imported stays on one filter line.
func CleanText(s string) string {
	return strings.TrimSpace(strings.Map(lineRune, s))
}

func lineRune(r rune) rune {
	if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == '\u2028' || r == '\u2029' {
		return ' '
	}
	return r
}

// freeGroupID returns an id no current group uses.
func (c *Config) freeGroupID() string {
	for i := 1; ; i++ {
		id := "g" + strconv.Itoa(i)
		taken := false
		for _, g := range c.ItemGroups {
			if g.ID == id {
				taken = true
				break
			}
		}
		if !taken {
			return id
		}
	}
}

// normalizeGroups drops broken groups, gives every group an id and a name, and
// keeps the count sane. It also clears styles left behind by deleted groups.
func (c *Config) normalizeGroups() {
	seen := map[string]bool{}
	out := c.ItemGroups[:0]
	for _, g := range c.ItemGroups {
		g.Name = CleanText(g.Name)
		g.Mode = g.GroupMode()
		g.Hide = g.Mode == ItemGroupModeHide
		if g.Mode != ItemGroupModeShow {
			g.Always = false
		}
		switch g.ThresholdUnit {
		case "exalted", "chaos", "divine":
		default:
			g.ThresholdUnit = "exalted"
		}
		if g.ThresholdValue < 0 {
			g.ThresholdValue = 0
		}
		items := g.Items[:0]
		for _, it := range g.Items {
			if it = CleanText(it); it != "" {
				items = append(items, it)
			}
		}
		g.Items = items
		if g.ID == "" || seen[g.ID] {
			g.ID = c.freeGroupID()
		}
		if g.Name == "" {
			g.Name = i18n.T("group.unnamed", len(out)+1)
		}
		seen[g.ID] = true
		out = append(out, g)
		if len(out) == MaxItemGroups {
			break
		}
	}
	c.ItemGroups = out

	live := map[string]bool{}
	for _, g := range c.ItemGroups {
		live[g.StyleKey()] = true
	}
	for key := range c.Styles {
		if strings.HasPrefix(key, UserGroupPrefix) && !live[key] {
			delete(c.Styles, key)
			delete(c.CustomStyles, key)
			delete(c.Sounds, key)
		}
	}
}

// migrateTiers turns the yes/no switches of earlier versions into the tier and
// level sliders that replaced them. A missing field means the user never had
// the switch, so the default stands.
func (c *Config) migrateTiers() {
	move := func(old *bool, target *int, on, off int) {
		if old == nil {
			return
		}
		if *old {
			*target = on
		} else {
			*target = off
		}
	}
	move(c.LegacyT5Rares, &c.T5RareTier, MaxRareTier, TierOff)
	// "T5 only" off used to mean every rare jewel was shown, which is tier 0.
	move(c.LegacyT5JewelsOnly, &c.RareJewelTier, MaxRareTier, 0)
	move(c.LegacyHighWaystones, &c.WaystoneTier, 14, TierOff)
	move(c.LegacyHighUncutGems, &c.UncutGemLevel, MaxUncutGemLevel, TierOff)
	if c.LegacyUncutSupportGems != nil {
		switch {
		case !*c.LegacyUncutSupportGems:
			c.UncutSupportLevel = TierHide // they were hidden outright
		case c.LegacyHighUncutGems != nil && *c.LegacyHighUncutGems:
			c.UncutSupportLevel = MaxSupportGemLevel // they followed the level rule
		default:
			c.UncutSupportLevel = 1 // shown at any level
		}
	}
	c.LegacyT5Rares, c.LegacyT5JewelsOnly = nil, nil
	c.LegacyHighWaystones, c.LegacyHighUncutGems, c.LegacyUncutSupportGems = nil, nil, nil
}

// clampTiers keeps every slider inside the range the game can produce.
func (c *Config) clampTiers() {
	clamp := func(v *int, min, max int) {
		if *v < min && *v != TierOff && *v != TierHide {
			*v = min
		}
		if *v > max {
			*v = max
		}
	}
	clamp(&c.T5RareTier, 0, MaxRareTier)
	clamp(&c.RareJewelTier, 0, MaxRareTier)
	clamp(&c.UncutGemLevel, 1, MaxUncutGemLevel)
	clamp(&c.UncutSupportLevel, 1, MaxSupportGemLevel)
	clamp(&c.WaystoneTier, 1, MaxWaystoneTier)
}

// migrateTierMeaning upgrades files written before the sliders grew a separate
// "hide" stop. Back then a single off position meant "no rule" for most
// sliders, but for rare jewels and support gems it actually hid them, so those
// two move to TierHide and keep behaving the way the user set them.
//
// Version 3: the "always show" list stopped being a spotlight and became
// "never hide, plainly". A look picked for the old meaning (often the loudest
// style with a sound) would make every cheap entry scream, so it is reset to
// the new plain default once.
//
// Version 4: the waystone slider hides the tiers below it instead of only
// highlighting the ones above. The old default (LegacyWaystoneDefault) goes to
// TierOff so nobody loses waystones they never chose to hide; any other tier
// was picked on purpose and keeps its value under the new meaning.
func (c *Config) migrateTierMeaning() {
	if c.ConfigVersion >= configVersion {
		c.ConfigVersion = configVersion
		return
	}
	if c.ConfigVersion < 2 {
		if c.RareJewelTier == TierOff {
			c.RareJewelTier = TierHide
		}
		if c.UncutSupportLevel == TierOff {
			c.UncutSupportLevel = TierHide
		}
	}
	if c.ConfigVersion < 3 {
		delete(c.Styles, GroupWhitelist)
		delete(c.Sounds, GroupWhitelist)
		delete(c.CustomStyles, GroupWhitelist)
	}
	if c.ConfigVersion < 4 && c.WaystoneTier == LegacyWaystoneDefault {
		c.WaystoneTier = TierOff
	}
	c.ConfigVersion = configVersion
}
