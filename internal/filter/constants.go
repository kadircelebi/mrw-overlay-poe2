package filter

// CategoryTheme defines visual highlight styling for drops in Path of Exile 2.
type CategoryTheme struct {
	Name      string
	Text      string
	Border    string
	BgT1      string
	BgT2      string
	Beam      string
	IconColor string
	IconShape string
}

// CategoryThemes holds color & effect definitions for all drop types.
var CategoryThemes = map[string]CategoryTheme{
	"currency": {
		Name:      "Currency",
		Text:      "255 215 0 255", // Bright Gold
		Border:    "255 225 0 255", // Neon Gold
		BgT1:      "60 45 5 255",
		BgT2:      "35 25 5 240",
		Beam:      "Yellow",
		IconColor: "Yellow",
		IconShape: "Star",
	},
	"runes": {
		Name:      "Runes & Augments",
		Text:      "50 230 255 255", // Electric Arcane Cyan
		Border:    "0 240 255 255",  // Neon Cyan
		BgT1:      "0 40 60 255",
		BgT2:      "0 25 40 240",
		Beam:      "Cyan",
		IconColor: "Cyan",
		IconShape: "Diamond",
	},
	"ritual": {
		Name:      "Ritual Omens",
		Text:      "255 80 80 255", // Blood Crimson / Red
		Border:    "255 30 70 255", // Neon Crimson Red
		BgT1:      "60 5 15 255",
		BgT2:      "40 5 10 240",
		Beam:      "Red",
		IconColor: "Red",
		IconShape: "Hexagon",
	},
	"essences": {
		Name:      "Essences",
		Text:      "225 120 255 255", // Mystical Violet / Purple
		Border:    "230 60 255 255",  // Neon Magenta
		BgT1:      "45 5 60 255",
		BgT2:      "30 5 45 240",
		Beam:      "Purple",
		IconColor: "Purple",
		IconShape: "Circle",
	},
	"ultimatum": {
		Name:      "Soul Cores (Ultimatum)",
		Text:      "255 140 0 255", // Hellfire Orange
		Border:    "255 120 0 255", // Neon Orange
		BgT1:      "60 20 0 255",
		BgT2:      "45 15 0 240",
		Beam:      "Orange",
		IconColor: "Orange",
		IconShape: "Pentagon",
	},
	"uncutgems": {
		Name:      "Uncut Skill & Support Gems",
		Text:      "80 255 160 255", // Radiant Emerald Green
		Border:    "0 255 130 255",  // Neon Emerald
		BgT1:      "5 50 20 255",
		BgT2:      "5 35 15 240",
		Beam:      "Green",
		IconColor: "Green",
		IconShape: "Triangle",
	},
	"lineagesupportgems": {
		Name:      "Lineage Support Gems",
		Text:      "120 255 200 255", // Light Jade Teal
		Border:    "50 255 190 255",
		BgT1:      "5 50 30 255",
		BgT2:      "5 40 25 240",
		Beam:      "Green",
		IconColor: "Green",
		IconShape: "Triangle",
	},
	"breach": {
		Name:      "Breach & Catalysts",
		Text:      "255 110 190 255", // Amethyst / Magenta Pink
		Border:    "255 60 190 255",  // Neon Pink
		BgT1:      "60 0 45 255",
		BgT2:      "40 0 30 240",
		Beam:      "Pink",
		IconColor: "Pink",
		IconShape: "Square",
	},
	"delirium": {
		Name:      "Delirium Distillates",
		Text:      "190 190 240 255", // Fog Grey / Ghost White
		Border:    "150 150 220 255",
		BgT1:      "30 30 50 255",
		BgT2:      "20 20 35 240",
		Beam:      "White",
		IconColor: "White",
		IconShape: "Kite",
	},
	"expedition": {
		Name:      "Expedition Sagas & Artifacts",
		Text:      "240 220 150 255", // Ancient Parchment / Sand
		Border:    "220 190 90 255",
		BgT1:      "50 40 10 255",
		BgT2:      "35 25 5 240",
		Beam:      "Brown",
		IconColor: "Brown",
		IconShape: "Cross",
	},
	"abyss": {
		Name:      "Abyssal Bones & Jawbones",
		Text:      "130 240 160 255", // Dark Abyss Green
		Border:    "70 200 110 255",
		BgT1:      "10 40 20 255",
		BgT2:      "5 25 10 240",
		Beam:      "Green",
		IconColor: "Green",
		IconShape: "Diamond",
	},
	"vaultkeys": {
		Name:      "Reliquary Keys",
		Text:      "255 180 50 255", // Ancient Bronze / Gold
		Border:    "255 140 0 255",
		BgT1:      "55 35 5 255",
		BgT2:      "35 20 0 240",
		Beam:      "Orange",
		IconColor: "Orange",
		IconShape: "Star",
	},
	"incursion": {
		Name:      "Incursion Orbs",
		Text:      "255 190 90 255", // Golden Sun
		Border:    "255 160 40 255",
		BgT1:      "50 35 5 255",
		BgT2:      "35 20 0 240",
		Beam:      "Yellow",
		IconColor: "Yellow",
		IconShape: "Hexagon",
	},
	"idol": {
		Name:      "Idols",
		Text:      "180 230 180 255", // Jade Stone
		Border:    "120 200 120 255",
		BgT1:      "20 45 20 255",
		BgT2:      "10 30 10 240",
		Beam:      "Green",
		IconColor: "Green",
		IconShape: "Circle",
	},
	"vaal": {
		Name:      "Vaal Infusers & Catalysts",
		Text:      "240 50 50 255", // Corruption Crimson
		Border:    "220 0 0 255",
		BgT1:      "65 0 0 255",
		BgT2:      "45 0 0 240",
		Beam:      "Red",
		IconColor: "Red",
		IconShape: "Hexagon",
	},
	"verisium": {
		Name:      "Verisium Ores",
		Text:      "200 210 255 255", // Metallic Platinum
		Border:    "160 180 240 255",
		BgT1:      "25 30 55 255",
		BgT2:      "15 20 40 240",
		Beam:      "White",
		IconColor: "White",
		IconShape: "Diamond",
	},
	"fragments": {
		Name:      "Fragments & Breachstones",
		Text:      "230 170 255 255", // Astral Violet
		Border:    "190 100 255 255",
		BgT1:      "40 10 60 255",
		BgT2:      "25 5 40 240",
		Beam:      "Purple",
		IconColor: "Purple",
		IconShape: "Pentagon",
	},
	"unique": {
		Name:      "Unique Items",
		Text:      "175 96 37 255", // Classic PoE Unique Orange/Brown
		Border:    "175 96 37 255",
		BgT1:      "50 20 5 255",
		BgT2:      "30 10 0 240",
		Beam:      "Orange",
		IconColor: "Orange",
		IconShape: "Hexagon",
	},
}

// CategoryLabels maps API category IDs to friendly display names.
var CategoryLabels = map[string]string{
	"currency":           "Currency",
	"runes":              "Runes & Augments",
	"ritual":             "Ritual Omens",
	"essences":           "Essences",
	"ultimatum":          "Soul Cores (Ultimatum)",
	"breach":             "Breach & Catalysts",
	"delirium":           "Delirium Distillates",
	"expedition":         "Expedition Sagas & Artifacts",
	"abyss":              "Abyssal Bones & Jawbones",
	"uncutgems":          "Uncut Skill & Support Gems",
	"lineagesupportgems": "Lineage Support Gems",
	"vaultkeys":          "Reliquary Keys",
	"incursion":          "Incursion Orbs",
	"idol":               "Idols",
	"vaal":               "Vaal Infusers & Catalysts",
	"verisium":           "Verisium Ores",
	"fragments":          "Fragments & Breachstones",
}

// NinjaUniqueTypes defines the endpoint categories on poe.ninja.
var NinjaUniqueTypes = []string{
	"UniqueArmours",
	"UniqueWeapons",
	"UniqueAccessories",
	"UniqueFlasks",
	"UniqueCharms",
	"UniqueJewels",
	"UniqueSanctumRelics",
	"UniqueTablets",
}

// EquipmentClasses subject to strict equipment filtering.
var EquipmentClasses = []string{
	"Amulets", "Belts", "Body Armours", "Boots", "Bows", "Bucklers", "Charms",
	"Crossbows", "Foci", "Gloves", "Helmets", "Jewels", "Life Flasks", "Mana Flasks",
	"One Hand Maces", "Quarterstaves", "Quivers", "Rings", "Sceptres", "Shields",
	"Spears", "Staves", "Talismans", "Two Hand Maces", "Wands",
}
