package overlay

import (
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

func TestGemFromTextPicksTheTooltipTitle(t *testing.T) {
	raw, ok := GemFromText(skillsPanelLines, 380, 480, someGemNames)
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

func TestGemFromTextForgivesAMisreadLetter(t *testing.T) {
	lines := []OcrLine{
		{Text: "URUK'S SMELT/NG", X: 100, Y: 100, W: 200, H: 30},
		{Text: "Support", X: 100, Y: 140, W: 90, H: 26},
	}
	raw, ok := GemFromText(lines, 0, 0, someGemNames)
	if !ok || !strings.Contains(raw, "\nUruk's Smelting\n") {
		t.Fatalf("got %q, %v", raw, ok)
	}
	if _, ok := GemFromText([]OcrLine{{Text: "MAP DEVICE", X: 1, Y: 1, W: 9, H: 9}}, 0, 0, someGemNames); ok {
		t.Fatal("matched a line that is no gem")
	}
}

func TestGemFromTextReadsASkillGemLevel(t *testing.T) {
	lines := []OcrLine{
		{Text: "ARCHMAGE", X: 500, Y: 300, W: 150, H: 30},
		{Text: "Buff, Persistent, Lightning", X: 500, Y: 340, W: 300, H: 26},
		{Text: "LEVEL: 18", X: 500, Y: 400, W: 120, H: 26},
	}
	raw, ok := GemFromText(lines, 480, 320, someGemNames)
	if !ok || raw != "Rarity: Gem\nArchmage\n--------\nBuff, Persistent, Lightning\nLevel: 18\n" {
		t.Fatalf("got %q", raw)
	}
}
