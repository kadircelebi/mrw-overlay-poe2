package filter

import "strings"

// Theme is a colour palette for a highlighted drop. Colours are "R G B A" as
// the filter language expects; Beam is also used for the minimap icon.
type Theme struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	BgColor   string `json:"bg"`
	TextColor string `json:"text"`
	Border    string `json:"border"`
	Beam      string `json:"beam"`
}

// ThemeList is the ordered list of selectable palettes. Every theme has its
// own background so they are told apart at a glance on the ground; a coloured
// border alone is too thin to notice in game.
var ThemeList = []Theme{
	{ID: "neon_cyan", Label: "Turkuaz zemin", BgColor: "0 200 230 255", TextColor: "0 0 0 255", Border: "255 255 255 255", Beam: "Cyan"},
	{ID: "neon_purple", Label: "Mor zemin", BgColor: "120 30 180 255", TextColor: "255 255 255 255", Border: "235 170 255 255", Beam: "Purple"},
	{ID: "neon_red", Label: "Kırmızı zemin", BgColor: "185 10 30 255", TextColor: "255 255 255 255", Border: "255 200 200 255", Beam: "Red"},
	{ID: "neon_gold", Label: "Altın zemin", BgColor: "255 200 40 255", TextColor: "40 20 0 255", Border: "255 255 255 255", Beam: "Yellow"},
	{ID: "neon_green", Label: "Yeşil zemin", BgColor: "20 160 70 255", TextColor: "255 255 255 255", Border: "190 255 210 255", Beam: "Green"},
	{ID: "dark", Label: "Koyu zemin, turkuaz çerçeve", BgColor: "15 15 25 255", TextColor: "255 255 255 255", Border: "0 255 255 255", Beam: "Cyan"},
	{ID: "gold", Label: "Koyu altın", BgColor: "60 45 5 255", TextColor: "255 215 0 255", Border: "255 215 0 255", Beam: "Yellow"},
	{ID: "classic_black", Label: "Beyaz zemin, siyah çerçeve", BgColor: "255 255 255 255", TextColor: "0 0 0 255", Border: "0 0 0 255", Beam: "White"},
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

// StyleGroup is a family of highlighted drops whose colours the user can
// change. Only colours change; size, sound and icon shape stay with the group
// so the importance ordering of the filter is preserved.
type StyleGroup struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Sample       string `json:"sample"` // item name shown in the preview
	DefaultLabel string `json:"defaultLabel"`
	Default      Theme  `json:"default"` // built-in colours
	AllowDefault bool   `json:"allowDefault"`
	FontSize     int    `json:"fontSize"`
	HasBeam      bool   `json:"hasBeam"`
	IconShape    string `json:"iconShape"` // "" when the group has no minimap icon
}

// Style group ids.
const (
	GroupDivine             = "divine"
	GroupCurrency           = "currency"
	GroupWhitelist          = "whitelist"
	GroupUnique             = "unique"
	GroupExceptional        = "exceptional"
	GroupExceptionalUnknown = "exceptional_unknown"
	GroupT5Rare             = "t5rare"
	GroupChance             = "chance"
)

// StyleGroups lists every customisable group in display order.
var StyleGroups = []StyleGroup{
	{ID: GroupDivine, Label: "Divine Orb", Sample: "Divine Orb",
		Default: themeByID["neon_cyan"], FontSize: 45, HasBeam: true, IconShape: "Star"},
	{ID: GroupCurrency, Label: "Değerli currency", Sample: "Perfect Exalted Orb", AllowDefault: true,
		DefaultLabel: "Varsayılan (kategori renkleri)", FontSize: 45, HasBeam: true, IconShape: "Star",
		Default: Theme{BgColor: "60 45 5 255", TextColor: "255 215 0 255", Border: "255 225 0 255", Beam: "Yellow"}},
	{ID: GroupWhitelist, Label: "Her zaman göster", Sample: "Mirror of Kalandra", AllowDefault: true,
		DefaultLabel: "Varsayılan (kırmızı, altın çerçeve)", FontSize: 45, HasBeam: true, IconShape: "Star",
		Default: Theme{BgColor: "180 0 0 255", TextColor: "255 255 255 255", Border: "255 215 0 255", Beam: "Red"}},
	{ID: GroupUnique, Label: "Değerli unique", Sample: "Utility Belt", AllowDefault: true,
		DefaultLabel: "Varsayılan (koyu kırmızı)", FontSize: 44, HasBeam: true, IconShape: "Star",
		Default: Theme{BgColor: "175 40 0 255", TextColor: "255 255 255 255", Border: "255 100 0 255", Beam: "Red"}},
	{ID: GroupExceptional, Label: "Değerli exceptional", Sample: "Exceptional Sekhema Sandals", AllowDefault: true,
		DefaultLabel: "Varsayılan (koyu mavi)", FontSize: 42, HasBeam: true, IconShape: "Diamond",
		Default: Theme{BgColor: "0 40 70 240", TextColor: "255 255 255 255", Border: "0 210 255 255", Beam: "Cyan"}},
	{ID: GroupExceptionalUnknown, Label: "Fiyatlanmamış exceptional", Sample: "Exceptional Cavalry Boots", AllowDefault: true,
		DefaultLabel: "Varsayılan (sönük mavi)", FontSize: 36,
		Default: Theme{BgColor: "0 25 45 220", TextColor: "200 230 255 255", Border: "0 150 200 255", Beam: "Cyan"}},
	{ID: GroupT5Rare, Label: "T5 rare", Sample: "Gold Ring", AllowDefault: true,
		DefaultLabel: "Varsayılan (koyu kahve, altın yazı)", FontSize: 40, IconShape: "Diamond",
		Default: Theme{BgColor: "40 25 0 255", TextColor: "255 215 0 255", Border: "255 180 0 255", Beam: "Yellow"}},
	{ID: GroupChance, Label: "Chance tabanları", Sample: "Heavy Belt", AllowDefault: true,
		DefaultLabel: "Varsayılan (koyu mavi, turkuaz yazı)", FontSize: 38, IconShape: "Circle",
		Default: Theme{BgColor: "10 30 50 240", TextColor: "0 240 255 255", Border: "0 200 255 255", Beam: "Cyan"}},
}

var groupByID = func() map[string]StyleGroup {
	m := make(map[string]StyleGroup, len(StyleGroups))
	for _, g := range StyleGroups {
		m[g.ID] = g
	}
	return m
}()

// Palette returns the colours for a group and whether the user overrode them.
func (c Config) Palette(group string) (Theme, bool) {
	g := groupByID[group]
	if t, ok := themeByID[c.Styles[group]]; ok {
		return t, true
	}
	return g.Default, false
}

// normalizeStyles drops unknown ids and migrates the old divine_theme field.
func (c *Config) normalizeStyles() {
	if c.Styles == nil {
		c.Styles = map[string]string{}
	}
	if _, ok := c.Styles[GroupDivine]; !ok && c.DivineTheme != "" {
		c.Styles[GroupDivine] = c.DivineTheme
	}
	for group, id := range c.Styles {
		g, known := groupByID[group]
		_, theme := themeByID[id]
		if !known || !(theme || (id == DefaultThemeID && g.AllowDefault)) {
			delete(c.Styles, group)
		}
	}
	c.DivineTheme = c.Styles[GroupDivine] // kept for older builds reading the file
}

// with returns a copy of st using the palette's colours. The beam and minimap
// icon are recoloured only if the group has them.
func (st *style) with(p Theme) *style {
	s := *st
	s.text, s.border, s.bg = p.TextColor, p.Border, p.BgColor
	if s.beam != "" {
		s.beam = p.Beam
	}
	if f := strings.Fields(s.icon); len(f) == 3 {
		s.icon = f[0] + " " + p.Beam + " " + f[2]
	}
	return &s
}
