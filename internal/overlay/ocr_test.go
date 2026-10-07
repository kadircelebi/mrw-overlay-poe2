package overlay

import (
	"slices"
	"strings"
	"testing"
)

// skillsPanelLines is what the Windows recognizer (Turkish) read off a
// Skills panel with Rakiata's Flow's tooltip open over the panel's rows.
var skillsPanelLines = []OcrLine{
	{Text: "SKILLS", X: 546, Y: 102, W: 110, H: 30},
	{Text: "VIRTUOUS BARRIER", X: 301, Y: 352, W: 250, H: 28},
	{Text: "LEVEL: 20", X: 642, Y: 356, W: 120, H: 26},
	{Text: "RAKIATA'S FLOW", X: 404, Y: 408, W: 252, H: 32},
	{Text: "Support", X: 402, Y: 454, W: 100, H: 26},
	{Text: "Lineage", X: 390, Y: 513, W: 96, H: 26},
	{Text: "CATEGORY: RAKIATA'S FLOW", X: 390, Y: 564, W: 396, H: 30},
	{Text: "COŞT MULTİPLIER: 120%", X: 390, Y: 611, W: 350, H: 30},
	{Text: "REQUIRES: LEVEL 65", X: 391, Y: 661, W: 280, H: 30},
	{Text: "ARCHMAGE", X: 301, Y: 1426, W: 150, H: 28},
	{Text: "LEVEL: 22", X: 643, Y: 1430, W: 120, H: 26},
}

var someGemNames = []string{"Archmage", "Virtuous Barrier", "Rakiata's Flow", "Uruk's Smelting", "Spell Totem"}

func TestScreenItemTextGemPicksTheTooltipTitle(t *testing.T) {
	raw, ok := ScreenItemText(skillsPanelLines, 380, 480, someGemNames, nil)
	if !ok {
		t.Fatal("no gem found")
	}
	want := "Rarity: Gem\nRakiata's Flow\n--------\nSupport\n"
	if raw != want {
		t.Fatalf("got %q, want %q", raw, want)
	}
	item, err := ParseItem(raw, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if item.BaseType != "Rakiata's Flow" || item.Class != "Support Gems" || !strings.EqualFold(item.Rarity, "gem") {
		t.Fatalf("parsed %q / %q / %q", item.BaseType, item.Class, item.Rarity)
	}
}

func TestScreenItemTextGemForgivesAMisreadLetter(t *testing.T) {
	lines := []OcrLine{
		{Text: "URUK'S SMELT/NG", X: 100, Y: 100, W: 200, H: 30},
		{Text: "Support", X: 100, Y: 140, W: 90, H: 26},
	}
	raw, ok := ScreenItemText(lines, 0, 0, someGemNames, nil)
	if !ok || !strings.Contains(raw, "\nUruk's Smelting\n") {
		t.Fatalf("got %q, %v", raw, ok)
	}
	if _, ok := ScreenItemText([]OcrLine{{Text: "MAP DEVICE", X: 1, Y: 1, W: 9, H: 9}}, 0, 0, someGemNames, nil); ok {
		t.Fatal("matched a line that is no gem")
	}
}

func TestScreenItemTextGemReadsASkillGemLevel(t *testing.T) {
	lines := []OcrLine{
		{Text: "ARCHMAGE", X: 500, Y: 300, W: 150, H: 30},
		{Text: "Buff, Persistent, Lightning", X: 500, Y: 340, W: 300, H: 26},
		{Text: "LEVEL: 18", X: 500, Y: 400, W: 120, H: 26},
	}
	raw, ok := ScreenItemText(lines, 480, 320, someGemNames, nil)
	if !ok || raw != "Rarity: Gem\nArchmage\n--------\nBuff, Persistent, Lightning\nLevel: 18\n" {
		t.Fatalf("got %q", raw)
	}
}

// runeshapeLines is what the Windows recognizer (Turkish) read off the left
// half of a 4K screen with Expedition's Runeshape Combinations open: "1x"
// comes out "IX", the title and the rune icons come out as noise.
var runeshapeLines = []OcrLine{
	{Text: "Ruwzshape COMİUÜOWS", X: 409, Y: 243, W: 342, H: 43},
	{Text: "IX Olroth's Saga", X: 807, Y: 402, W: 265, H: 39},
	{Text: "IX Vorana's Saga", X: 795, Y: 564, W: 277, H: 39},
	{Text: "IX Uhtred's Saga", X: 799, Y: 726, W: 274, H: 39},
	{Text: "IX Medved's Saga", X: 783, Y: 888, W: 289, H: 39},
	{Text: "3x Artificer's Orb", X: 784, Y: 976, W: 287, H: 33},
	{Text: "IX Greater Robust Rune", X: 672, Y: 1071, W: 398, H: 30},
	{Text: "ZŞ89", X: 118, Y: 1147, W: 226, H: 68},
	{Text: "IX Greater Resolve Rune", X: 664, Y: 1166, W: 407, H: 30},
	{Text: "IX Greater Adept Rune", X: 691, Y: 1261, W: 379, H: 39},
	{Text: "Life", X: 76, Y: 1590, W: 56, H: 30},
}

var someCurrencyNames = []string{"Olroth's Saga", "Vorana's Saga", "Uhtred's Saga", "Medved's Saga", "Artificer's Orb",
	"Greater Robust Rune", "Greater Resolve Rune", "Greater Adept Rune", "Chaos Orb"}

// withTooltip adds the tooltip the game opens beside a hovered row: its
// title without a count, and a line of text under it.
func withTooltip(title string, y float64) []OcrLine {
	return append(slices.Clone(runeshapeLines),
		OcrLine{Text: title, X: 2330, Y: y, W: 450, H: 55},
		OcrLine{Text: "While this item is active in your inventory", X: 1820, Y: y + 100, W: 1400, H: 45})
}

func TestScreenItemTextRuneshapeRowUnderTheCursor(t *testing.T) {
	// No tooltip: the row nearest the cursor, its count as the stack.
	raw, ok := ScreenItemText(runeshapeLines, 900, 990, someGemNames, someCurrencyNames)
	want := "Item Class: Stackable Currency\nRarity: Currency\nArtificer's Orb\n--------\nStack Size: 3/3\n"
	if !ok || raw != want {
		t.Fatalf("got %q, %v; want %q", raw, ok, want)
	}
	item, err := ParseItem(raw, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if item.BaseType != "Artificer's Orb" || item.Rarity != "currency" || item.StackSize != 3 {
		t.Fatalf("parsed %q / %q / %d", item.BaseType, item.Rarity, item.StackSize)
	}
}

func TestScreenItemTextRuneshapeTooltipWins(t *testing.T) {
	// The cursor on a two-line row's icons sits about as near the row above;
	// the tooltip's title settles it.
	raw, ok := ScreenItemText(withTooltip("MEDVED'S SAGA", 800), 300, 829, someGemNames, someCurrencyNames)
	if !ok || raw != "Item Class: Stackable Currency\nRarity: Currency\nMedved's Saga\n--------\n" {
		t.Fatalf("got %q, %v", raw, ok)
	}
	// The count comes from the row of that name.
	raw, ok = ScreenItemText(withTooltip("ARTİFİCER'S ORB", 900), 300, 990, someGemNames, someCurrencyNames)
	if !ok || !strings.Contains(raw, "\nArtificer's Orb\n") || !strings.Contains(raw, "Stack Size: 3/3") {
		t.Fatalf("got %q, %v", raw, ok)
	}
}

func TestScreenItemTextRuneshapeFarFromTheCursor(t *testing.T) {
	// Below the panel, on the life text under it, and out on the map at a
	// row's height: no row is under the cursor.
	for _, at := range [][2]float64{{900, 2100}, {100, 1605}, {2600, 990}} {
		if raw, ok := ScreenItemText(runeshapeLines, at[0], at[1], someGemNames, someCurrencyNames); ok {
			t.Errorf("cursor %v matched %q", at, raw)
		}
	}
}

func TestSplitCountReadsTheMisreadDigits(t *testing.T) {
	for text, want := range map[string]int{"IX Chaos Orb": 1, "IOX Chaos Orb": 10, "3x Chaos Orb": 3, "3)' Chaos Orb": 3, "12x Chaos Orb": 12, "Chaos Orb": 1} {
		if got, rest := splitCount(text); got != want || rest != "Chaos Orb" {
			t.Errorf("%q: %d %q, want %d", text, got, rest, want)
		}
	}
}

func TestScreenItemTextGemBesideCurrencyNames(t *testing.T) {
	raw, ok := ScreenItemText(skillsPanelLines, 380, 480, someGemNames, someCurrencyNames)
	if !ok || !strings.Contains(raw, "\nRakiata's Flow\n") {
		t.Fatalf("got %q, %v", raw, ok)
	}
}

// alloyLines is a second panel (2000 px wide capture) with Prismatic Alloy's
// tooltip open: "3x" came out "3)'" on one row and as "M" on another.
var alloyLines = []OcrLine{
	{Text: "Runeshape COMVinaü0M", X: 250, Y: 157, W: 239, H: 30},
	{Text: "IX Prismatic Alloy", X: 505, Y: 216, W: 209, H: 28},
	{Text: "Rare Unique item", X: 503, Y: 285, W: 209, H: 25},
	{Text: "3)' Greater Orb of Transmutation", X: 332, Y: 349, W: 380, H: 23},
	{Text: "M Greater Orb of Augmentation", X: 41, Y: 402, W: 671, H: 50},
	{Text: "PRISMATIC ALLOY", X: 1247, Y: 106, W: 228, H: 23},
	{Text: "REMOVES A RÂNDOM AND AUGÜENTS XRARE İTEM WITH A NEW GUARANTEED MODIFIER", X: 807, Y: 152, W: 1108, H: 27},
	{Text: "GLOVES: DAMAGE ELEMENTAL RESISTANCES", X: 1003, Y: 186, W: 714, H: 26},
}

func TestScreenItemTextRuneshapeMisreadCounts(t *testing.T) {
	names := append(slices.Clone(someCurrencyNames), "Prismatic Alloy", "Greater Orb of Transmutation", "Greater Orb of Augmentation")
	// The capitals title is the tooltip; the "3)'" row near the cursor is not.
	raw, ok := ScreenItemText(alloyLines, 300, 228, someGemNames, names)
	if !ok || raw != "Item Class: Stackable Currency\nRarity: Currency\nPrismatic Alloy\n--------\n" {
		t.Fatalf("got %q, %v", raw, ok)
	}
	rows := slices.DeleteFunc(slices.Clone(alloyLines), func(l OcrLine) bool { return isCapsTitle(l.Text) })
	raw, ok = ScreenItemText(rows, 300, 360, someGemNames, names)
	if !ok || !strings.Contains(raw, "\nGreater Orb of Transmutation\n") || !strings.Contains(raw, "Stack Size: 3/3") {
		t.Fatalf("3)' row: got %q, %v", raw, ok)
	}
	// The count read as "M" is lost, the name is not.
	raw, ok = ScreenItemText(rows, 300, 427, someGemNames, names)
	if !ok || !strings.Contains(raw, "\nGreater Orb of Augmentation\n") {
		t.Fatalf("M row: got %q, %v", raw, ok)
	}
	// On the icons left of the "3)'" row: the "M" row's box reaches over its
	// icons, so its centre is nearer sideways, but the height decides.
	raw, ok = ScreenItemText(rows, 142, 360, someGemNames, names)
	if !ok || !strings.Contains(raw, "\nGreater Orb of Transmutation\n") {
		t.Fatalf("icons of the 3)' row: got %q, %v", raw, ok)
	}
	// A reward that is no currency prices nothing, not the row beside it.
	if raw, ok := ScreenItemText(rows, 300, 297, someGemNames, names); ok {
		t.Fatalf("Rare Unique Item row: got %q", raw)
	}
}
