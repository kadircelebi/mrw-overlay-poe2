package filter

// DivineThemeStyle contains colors and beam effects for the spotlight Divine Orb drop.
type DivineThemeStyle struct {
	BgColor   string
	TextColor string
	Border    string
	Beam      string
	IconColor string
}

// DivineThemes maps theme names to their visual rule declarations.
var DivineThemes = map[string]DivineThemeStyle{
	"neon_cyan": {
		BgColor:   "255 255 255 255",
		TextColor: "0 0 0 255",
		Border:    "0 255 255 255",
		Beam:      "Cyan",
		IconColor: "Cyan",
	},
	"neon_purple": {
		BgColor:   "255 255 255 255",
		TextColor: "0 0 0 255",
		Border:    "220 50 255 255",
		Beam:      "Purple",
		IconColor: "Purple",
	},
	"neon_red": {
		BgColor:   "255 255 255 255",
		TextColor: "0 0 0 255",
		Border:    "255 30 70 255",
		Beam:      "Red",
		IconColor: "Red",
	},
	"neon_gold": {
		BgColor:   "255 255 255 255",
		TextColor: "0 0 0 255",
		Border:    "255 215 0 255",
		Beam:      "Yellow",
		IconColor: "Yellow",
	},
	"neon_green": {
		BgColor:   "255 255 255 255",
		TextColor: "0 0 0 255",
		Border:    "0 255 120 255",
		Beam:      "Green",
		IconColor: "Green",
	},
	"dark": {
		BgColor:   "15 15 25 255",
		TextColor: "255 255 255 255",
		Border:    "0 255 255 255",
		Beam:      "Cyan",
		IconColor: "Cyan",
	},
	"gold": {
		BgColor:   "60 45 5 255",
		TextColor: "255 215 0 255",
		Border:    "255 215 0 255",
		Beam:      "Yellow",
		IconColor: "Yellow",
	},
	"classic_black": {
		BgColor:   "255 255 255 255",
		TextColor: "0 0 0 255",
		Border:    "0 0 0 255",
		Beam:      "White",
		IconColor: "White",
	},
}
