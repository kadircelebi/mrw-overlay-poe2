package filter

// DivineThemeStyle contains colors and beam effects for the spotlight Divine Orb drop.
// Colours are "R G B A" as the filter language expects.
type DivineThemeStyle struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	BgColor   string `json:"bg"`
	TextColor string `json:"text"`
	Border    string `json:"border"`
	Beam      string `json:"beam"` // PlayEffect / minimap icon colour name
	IconColor string `json:"icon"`
}

// DivineThemeList is the ordered list shown in the UI. Every theme has its own
// background so they are told apart at a glance on the ground; a coloured
// border alone is too thin to notice in game.
var DivineThemeList = []DivineThemeStyle{
	{ID: "neon_cyan", Label: "Turkuaz zemin", BgColor: "0 200 230 255", TextColor: "0 0 0 255",
		Border: "255 255 255 255", Beam: "Cyan", IconColor: "Cyan"},
	{ID: "neon_purple", Label: "Mor zemin", BgColor: "120 30 180 255", TextColor: "255 255 255 255",
		Border: "235 170 255 255", Beam: "Purple", IconColor: "Purple"},
	{ID: "neon_red", Label: "Kırmızı zemin", BgColor: "185 10 30 255", TextColor: "255 255 255 255",
		Border: "255 200 200 255", Beam: "Red", IconColor: "Red"},
	{ID: "neon_gold", Label: "Altın zemin", BgColor: "255 200 40 255", TextColor: "40 20 0 255",
		Border: "255 255 255 255", Beam: "Yellow", IconColor: "Yellow"},
	{ID: "neon_green", Label: "Yeşil zemin", BgColor: "20 160 70 255", TextColor: "255 255 255 255",
		Border: "190 255 210 255", Beam: "Green", IconColor: "Green"},
	{ID: "dark", Label: "Koyu zemin, turkuaz çerçeve", BgColor: "15 15 25 255", TextColor: "255 255 255 255",
		Border: "0 255 255 255", Beam: "Cyan", IconColor: "Cyan"},
	{ID: "gold", Label: "Koyu altın", BgColor: "60 45 5 255", TextColor: "255 215 0 255",
		Border: "255 215 0 255", Beam: "Yellow", IconColor: "Yellow"},
	{ID: "classic_black", Label: "Beyaz zemin, siyah çerçeve", BgColor: "255 255 255 255", TextColor: "0 0 0 255",
		Border: "0 0 0 255", Beam: "White", IconColor: "White"},
}

// DivineThemes maps theme ids to their styles.
var DivineThemes = func() map[string]DivineThemeStyle {
	m := make(map[string]DivineThemeStyle, len(DivineThemeList))
	for _, t := range DivineThemeList {
		m[t.ID] = t
	}
	return m
}()
