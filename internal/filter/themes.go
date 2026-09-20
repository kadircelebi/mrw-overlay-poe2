package filter

import (
	"fmt"
	"regexp"
	"strings"

	"poe2filter/internal/i18n"
)

// Theme is a colour palette for a highlighted drop. Colours are "R G B A" as
// the filter language expects (empty = the game's default).
type Theme struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	BgColor   string `json:"bg"`
	TextColor string `json:"text"`
	Border    string `json:"border"`
	Beam      string `json:"beam"`  // PlayEffect, e.g. "Red" or "Purple Temp"
	Icon      string `json:"icon"`  // minimap icon colour; "" = the beam colour
	Shape     string `json:"shape"` // minimap icon shape; "" = the group's shape
	// Full themes (NeverSink and custom) apply beam and icon exactly as given,
	// adding or removing them; the built-in palettes only recolour the
	// group's own beam and icon.
	Full     bool   `json:"full"`
	Category string `json:"category,omitempty"` // NeverSink section, e.g. "currency"
	Count    int    `json:"count,omitempty"`    // NeverSink rules using this style
}

// Theme id prefixes and special ids stored in Config.Styles.
const (
	NeverSinkThemePrefix = "ns:"
	CustomThemeID        = "custom"
)

// Colours and shapes accepted in custom styles: exactly those NeverSink's
// PoE2 filter uses, so every value is known to be valid in game.
var (
	EffectColours = []string{"Blue", "Brown", "Cyan", "Green", "Grey", "Orange", "Pink", "Purple", "Red", "White", "Yellow"}
	IconShapes    = []string{"Circle", "Cross", "Diamond", "Hexagon", "Kite", "Pentagon", "Square", "Star", "Triangle", "UpsideDownHouse"}
)

// CustomStyle is a user-defined look for one group. Colours are "#rrggbb";
// empty Beam/Icon/Shape mean "none".
type CustomStyle struct {
	Bg     string `json:"bg"`
	Text   string `json:"text"`
	Border string `json:"border"`
	Beam   string `json:"beam"`
	Icon   string `json:"icon"`
	Shape  string `json:"shape"`
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func oneOf(v string, list []string) bool {
	for _, x := range list {
		if v == x {
			return true
		}
	}
	return false
}

func (cs CustomStyle) valid() bool {
	for _, c := range []string{cs.Bg, cs.Text, cs.Border} {
		if !hexColour.MatchString(c) {
			return false
		}
	}
	return (cs.Beam == "" || oneOf(cs.Beam, EffectColours)) &&
		(cs.Icon == "" || oneOf(cs.Icon, EffectColours)) &&
		(cs.Shape == "" || oneOf(cs.Shape, IconShapes)) &&
		(cs.Shape == "" || cs.Icon != "")
}

func hexToRGBA(h string) string {
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(h, "#"), "%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("%d %d %d 255", r, g, b)
}

func (cs CustomStyle) theme() Theme {
	return Theme{ID: CustomThemeID, Label: i18n.T("theme.custom"), Full: true,
		BgColor: hexToRGBA(cs.Bg), TextColor: hexToRGBA(cs.Text), Border: hexToRGBA(cs.Border),
		Beam: cs.Beam, Icon: cs.Icon, Shape: cs.Shape}
}

// ThemeList is the ordered list of selectable palettes. Every theme has its
// own background so they are told apart at a glance on the ground; a coloured
// border alone is too thin to notice in game.
var ThemeList = []Theme{
	{ID: "neon_cyan", BgColor: "0 200 230 255", TextColor: "0 0 0 255", Border: "255 255 255 255", Beam: "Cyan"},
	{ID: "neon_purple", BgColor: "120 30 180 255", TextColor: "255 255 255 255", Border: "235 170 255 255", Beam: "Purple"},
	{ID: "neon_red", BgColor: "185 10 30 255", TextColor: "255 255 255 255", Border: "255 200 200 255", Beam: "Red"},
	{ID: "neon_gold", BgColor: "255 200 40 255", TextColor: "40 20 0 255", Border: "255 255 255 255", Beam: "Yellow"},
	{ID: "neon_green", BgColor: "20 160 70 255", TextColor: "255 255 255 255", Border: "190 255 210 255", Beam: "Green"},
	{ID: "dark", BgColor: "15 15 25 255", TextColor: "255 255 255 255", Border: "0 255 255 255", Beam: "Cyan"},
	{ID: "gold", BgColor: "60 45 5 255", TextColor: "255 215 0 255", Border: "255 215 0 255", Beam: "Yellow"},
	{ID: "classic_black", BgColor: "255 255 255 255", TextColor: "0 0 0 255", Border: "0 0 0 255", Beam: "White"},
}

var themeByID = func() map[string]Theme {
	m := make(map[string]Theme, len(ThemeList))
	for _, t := range ThemeList {
		m[t.ID] = t
	}
	return m
}()

// DefaultThemeID selects a group's built-in look.
const DefaultThemeID = "default"

// StyleGroup is a family of highlighted drops whose colours and sound the
// user can change. Size and icon shape stay with the group so the importance
// ordering of the filter is preserved.
type StyleGroup struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Sample       string `json:"sample"` // item name shown in the preview
	DefaultLabel string `json:"defaultLabel"`
	Default      Theme  `json:"default"` // built-in colours
	AllowDefault bool   `json:"allowDefault"`
	DefaultSound string `json:"defaultSound"` // PlayAlertSound id, "" = silent
	FontSize     int    `json:"fontSize"`
	HasBeam      bool   `json:"hasBeam"`
	IconShape    string `json:"iconShape"` // "" when the group has no minimap icon
}

// Style group ids.
const (
	GroupDivine             = "divine"
	GroupCurrency           = "currency"
	GroupWhitelist          = "whitelist"
	GroupWhitelistMid       = "whitelist_mid"
	GroupUnique             = "unique"
	GroupExceptional        = "exceptional"
	GroupExceptionalUnknown = "exceptional_unknown"
	GroupT5Rare             = "t5rare"
	GroupChance             = "chance"
)

// StyleGroups lists every customisable group in display order.
var StyleGroups = []StyleGroup{
	{ID: GroupDivine, Sample: "Divine Orb",
		Default: themeByID["neon_cyan"], DefaultSound: "6", FontSize: 45, HasBeam: true, IconShape: "Star"},
	{ID: GroupCurrency, Sample: "Perfect Exalted Orb", AllowDefault: true,
		DefaultSound: "6", FontSize: 45, HasBeam: true, IconShape: "Star",
		Default: Theme{BgColor: "60 45 5 255", TextColor: "255 215 0 255", Border: "255 225 0 255", Beam: "Yellow"}},
	{ID: GroupWhitelist, Sample: "Mirror of Kalandra", AllowDefault: true,
		DefaultSound: "6", FontSize: 45, HasBeam: true, IconShape: "Star",
		Default: Theme{BgColor: "180 0 0 255", TextColor: "255 255 255 255", Border: "255 215 0 255", Beam: "Red"}},
	{ID: GroupWhitelistMid, Sample: "Orb of Annulment", AllowDefault: true,
		DefaultSound: "2", FontSize: 40, IconShape: "Diamond",
		Default: Theme{BgColor: "70 20 100 230", TextColor: "240 220 255 255", Border: "180 120 255 255", Beam: "Purple"}},
	{ID: GroupUnique, Sample: "Utility Belt", AllowDefault: true,
		DefaultSound: "6", FontSize: 44, HasBeam: true, IconShape: "Star",
		Default: Theme{BgColor: "175 40 0 255", TextColor: "255 255 255 255", Border: "255 100 0 255", Beam: "Red"}},
	{ID: GroupExceptional, Sample: "Exceptional Sekhema Sandals", AllowDefault: true,
		DefaultSound: "2", FontSize: 42, HasBeam: true, IconShape: "Diamond",
		Default: Theme{BgColor: "0 40 70 240", TextColor: "255 255 255 255", Border: "0 210 255 255", Beam: "Cyan"}},
	{ID: GroupExceptionalUnknown, Sample: "Exceptional Cavalry Boots", AllowDefault: true,
		FontSize: 36,
		Default:  Theme{BgColor: "0 25 45 220", TextColor: "200 230 255 255", Border: "0 150 200 255", Beam: "Cyan"}},
	{ID: GroupT5Rare, Sample: "Gold Ring", AllowDefault: true,
		FontSize: 40, IconShape: "Diamond",
		Default: Theme{BgColor: "40 25 0 255", TextColor: "255 215 0 255", Border: "255 180 0 255", Beam: "Yellow"}},
	{ID: GroupChance, Sample: "Heavy Belt", AllowDefault: true,
		FontSize: 38, IconShape: "Circle",
		Default: Theme{BgColor: "10 30 50 240", TextColor: "0 240 255 255", Border: "0 200 255 255", Beam: "Cyan"}},
}

// UserGroupPrefix marks the Styles/Sounds keys of a user-made item group, so
// they can never collide with the built-in group ids.
const UserGroupPrefix = "user:"

// UserGroupTemplate is the look a new user group starts from: the medium
// highlight, which is deliberately weaker than the spotlight list.
func UserGroupTemplate() StyleGroup {
	g := groupByID[GroupWhitelistMid]
	g.ID, g.Label, g.DefaultLabel = "", "", i18n.T("groupDefault.userGroup")
	return g
}

// styleGroup resolves a Styles key to its definition; user groups all share
// the template's defaults.
func styleGroup(key string) StyleGroup {
	if g, ok := groupByID[key]; ok {
		return g
	}
	if strings.HasPrefix(key, UserGroupPrefix) {
		return groupByID[GroupWhitelistMid]
	}
	return StyleGroup{}
}

var groupByID = func() map[string]StyleGroup {
	m := make(map[string]StyleGroup, len(StyleGroups))
	for _, g := range StyleGroups {
		m[g.ID] = g
	}
	return m
}()

// Palette returns the colours for a group and whether the user overrode them.
// ns holds the NeverSink styles of the current base filter (may be nil).
func (c Config) Palette(group string, ns map[string]Theme) (Theme, bool) {
	g := styleGroup(group)
	id := c.Styles[group]
	switch {
	case id == CustomThemeID:
		if cs, ok := c.CustomStyles[group]; ok {
			return cs.theme(), true
		}
	case strings.HasPrefix(id, NeverSinkThemePrefix):
		if t, ok := ns[strings.TrimPrefix(id, NeverSinkThemePrefix)]; ok {
			return t, true
		}
	default:
		if t, ok := themeByID[id]; ok {
			return t, true
		}
	}
	return g.Default, false
}

// NeverSinkPreset maps each group to the NeverSink style closest in spirit.
var NeverSinkPreset = map[string]string{
	GroupDivine:             "apex_stier",
	GroupCurrency:           "currency_a",
	GroupWhitelist:          "apex_stier",
	GroupWhitelistMid:       "currency_c",
	GroupUnique:             "uniques_a",
	GroupExceptional:        "exotics_btier",
	GroupExceptionalUnknown: "exotics_ctier",
	GroupT5Rare:             "gear_tieredjewellery",
	GroupChance:             "itemproperty_achancing",
}

var nsTag = regexp.MustCompile(`^[a-z0-9_]+$`)

// Sound choices: "" (group default), "none", "1".."6" (game sounds) or
// "file:<name>" for a sound file in the filter folder.
const (
	SoundDefault    = ""
	SoundNone       = "none"
	SoundFilePrefix = "file:"
)

// GameSounds are the PlayAlertSound ids NeverSink's PoE2 filter uses.
var GameSounds = []string{"1", "2", "3", "4", "5", "6"}

// validSound reports whether v is an acceptable sound choice.
func validSound(v string) bool {
	if v == SoundDefault || v == SoundNone {
		return true
	}
	if name, ok := strings.CutPrefix(v, SoundFilePrefix); ok {
		return name != "" && !strings.ContainsAny(name, `"/\:`)
	}
	for _, s := range GameSounds {
		if v == s {
			return true
		}
	}
	return false
}

// normalizeStyles drops unknown ids and migrates the old divine_theme field.
func (c *Config) normalizeStyles() {
	// Keys are the built-in groups plus the user's own groups; anything else is
	// left over from a group that has been deleted.
	known := func(key string) (StyleGroup, bool) {
		if g, ok := groupByID[key]; ok {
			return g, true
		}
		for _, ug := range c.ItemGroups {
			if ug.StyleKey() == key {
				return groupByID[GroupWhitelistMid], true
			}
		}
		return StyleGroup{}, false
	}
	if c.Styles == nil {
		c.Styles = map[string]string{}
	}
	if _, ok := c.Styles[GroupDivine]; !ok && c.DivineTheme != "" {
		c.Styles[GroupDivine] = c.DivineTheme
	}
	if c.CustomStyles == nil {
		c.CustomStyles = map[string]CustomStyle{}
	}
	for group, cs := range c.CustomStyles {
		if _, ok := known(group); !ok || !cs.valid() {
			delete(c.CustomStyles, group)
		}
	}
	for group, id := range c.Styles {
		g, isKnown := known(group)
		_, theme := themeByID[id]
		_, custom := c.CustomStyles[group]
		ok := theme || (id == DefaultThemeID && g.AllowDefault) ||
			(id == CustomThemeID && custom) ||
			(strings.HasPrefix(id, NeverSinkThemePrefix) && nsTag.MatchString(strings.TrimPrefix(id, NeverSinkThemePrefix)))
		if !isKnown || !ok {
			delete(c.Styles, group)
		}
	}
	c.DivineTheme = c.Styles[GroupDivine] // kept for older builds reading the file

	if c.Sounds == nil {
		c.Sounds = map[string]string{}
	}
	// v1 stored the Divine sound separately ("auto", "1", "6" or a file).
	if _, ok := c.Sounds[GroupDivine]; !ok && c.DivineSound != "" {
		switch ds := c.DivineSound; {
		case ds == "auto":
		case validSound(ds):
			c.Sounds[GroupDivine] = ds
		case strings.HasSuffix(strings.ToLower(ds), ".mp3") || strings.HasSuffix(strings.ToLower(ds), ".wav"):
			c.Sounds[GroupDivine] = SoundFilePrefix + ds
		}
	}
	for group, v := range c.Sounds {
		if _, ok := known(group); !ok || !validSound(v) || v == SoundDefault {
			delete(c.Sounds, group)
		}
	}
	c.DivineSound = ""
}

// Sound returns the effective sound choice for a group.
func (c Config) Sound(group string) string { return c.Sounds[group] }

// withSound applies a sound choice to a copy of st (the group default when v
// is empty).
func (st *style) withSound(v string) *style {
	s := *st
	switch {
	case v == SoundDefault:
	case v == SoundNone:
		s.sound, s.custom = "", ""
	case strings.HasPrefix(v, SoundFilePrefix):
		s.sound, s.custom = "", strings.TrimPrefix(v, SoundFilePrefix)
	default:
		s.sound, s.custom = v+" 300", ""
	}
	return &s
}

// with returns a copy of st using the palette's colours. The beam and minimap
// icon are recoloured only if the group has them.
func (st *style) with(p Theme) *style {
	s := *st
	s.text, s.border, s.bg = p.TextColor, p.Border, p.BgColor
	iconColour := p.Icon
	if iconColour == "" {
		iconColour, _, _ = strings.Cut(p.Beam, " ") // "Purple Temp" -> "Purple"
	}
	size, shape := "", ""
	if f := strings.Fields(s.icon); len(f) == 3 {
		size, shape = f[0], f[2]
	}
	if p.Full {
		s.beam = p.Beam
		s.icon = ""
		if p.Shape != "" && iconColour != "" {
			if size == "" {
				size = "1"
			}
			s.icon = size + " " + iconColour + " " + p.Shape
		}
		return &s
	}
	if s.beam != "" {
		s.beam = p.Beam
	}
	if size != "" {
		if p.Shape != "" {
			shape = p.Shape
		}
		s.icon = size + " " + iconColour + " " + shape
	}
	return &s
}

// groupLabelKeys ties each style group to its translation keys: the group name
// and, when the group can fall back to the built-in look, the name of that
// default. Keeping them in one table makes a forgotten key easy to spot.
var groupLabelKeys = map[string][2]string{
	GroupDivine:             {"group.divine", ""},
	GroupCurrency:           {"group.currency", "groupDefault.currency"},
	GroupWhitelist:          {"group.whitelist", "groupDefault.whitelist"},
	GroupWhitelistMid:       {"group.whitelistMid", "groupDefault.whitelistMid"},
	GroupUnique:             {"group.unique", "groupDefault.unique"},
	GroupExceptional:        {"group.exceptional", "groupDefault.exceptional"},
	GroupExceptionalUnknown: {"group.exceptionalUnknown", "groupDefault.exceptionalUnknown"},
	GroupT5Rare:             {"group.t5rare", "groupDefault.t5rare"},
	GroupChance:             {"group.chance", "groupDefault.chance"},
}

// LocalizedThemes returns the selectable palettes with their names in the
// active interface language.
func LocalizedThemes() []Theme {
	out := make([]Theme, len(ThemeList))
	copy(out, ThemeList)
	for i := range out {
		out[i].Label = i18n.T("theme." + out[i].ID)
	}
	return out
}

// LocalizedStyleGroups returns the customisable groups with their names in the
// active interface language.
// The medium list became a user group, so its entry only survives as the
// template new user groups start from; the panel must not offer it as a tab.
func LocalizedStyleGroups() []StyleGroup {
	out := make([]StyleGroup, 0, len(StyleGroups))
	for _, g := range StyleGroups {
		if g.ID != GroupWhitelistMid {
			out = append(out, g)
		}
	}
	for i := range out {
		keys := groupLabelKeys[out[i].ID]
		out[i].Label = i18n.T(keys[0])
		if keys[1] != "" {
			out[i].DefaultLabel = i18n.T(keys[1])
		}
	}
	return out
}
