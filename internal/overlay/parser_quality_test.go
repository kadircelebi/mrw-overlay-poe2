package overlay

import (
	"slices"
	"strings"
	"testing"
)

const catalystClipboardItem = `Item Class: Amulets
Rarity: Rare
Miracle Braid
Gold Amulet
--------
Quality (Caster Modifiers): +40% (augmented)
--------
Requires: Level 65
--------
Item Level: 84
--------
{ Enhancement }
Allocates Thaumaturgic Generator — Unscalable Value
--------
{ Implicit Modifier }
20(12-20)% increased Rarity of Items found
--------
{ Fractured Prefix Modifier "Countess'" (Tier: 1) }
+50(47-50) to Spirit
{ Prefix Modifier "Ultramarine" (Tier: 1) — Mana }
+185(180-189) to maximum Mana
{ Prefix Modifier "Mnemonic" (Tier: 1) — Mana }
8(7-8)% increased maximum Mana
{ Suffix Modifier "of the Sorcerer" (Tier: 1) — Caster, Gem — 40% Increased }
+3 to Level of all Spell Skills
{ Suffix Modifier "of Prestidigitation" (Tier: 1) — Caster, Speed — 40% Increased }
26(25-28)% increased Cast Speed
{ Desecrated Suffix Modifier "of Kurgal" (Tier: 1) — Gem }
+5(3-5)% to Quality of all Skills
--------
Fractured Item`

func catalystClipboardCatalog() Catalog {
	return Catalog{Stats: []StatGroup{
		{ID: "explicit", Entries: []StatEntry{
			{ID: "explicit.spell", Text: "# to Level of all Spell Skills", Type: "explicit"},
			{ID: "explicit.speed", Text: "#% increased Cast Speed", Type: "explicit"},
			{ID: "explicit.mana", Text: "# to maximum Mana", Type: "explicit"},
			{ID: "explicit.mana_percent", Text: "#% increased maximum Mana", Type: "explicit"},
		}},
		{ID: "fractured", Entries: []StatEntry{
			{ID: "fractured.spirit", Text: "# to Spirit", Type: "fractured"},
		}},
		{ID: "desecrated", Entries: []StatEntry{
			{ID: "desecrated.quality", Text: "#% to Quality of all Skills", Type: "desecrated"},
		}},
	}}
}

func TestClipboardModifierMagnitude(t *testing.T) {
	item, err := ParseItem(catalystClipboardItem, catalystClipboardCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if item.Raw != catalystClipboardItem || item.Quality != 40 {
		t.Fatal("source text or item quality changed")
	}
	want := map[string]struct {
		text  string
		value float64
	}{
		"explicit.spell":        {"+4 to Level of all Spell Skills", 4},
		"explicit.speed":        {"36(35-39)% increased Cast Speed", 36},
		"explicit.mana":         {"+185(180-189) to maximum Mana", 185},
		"explicit.mana_percent": {"8(7-8)% increased maximum Mana", 8},
		"fractured.spirit":      {"+50(47-50) to Spirit", 50},
		"desecrated.quality":    {"+5(3-5)% to Quality of all Skills", 5},
	}
	for _, mod := range item.Mods {
		if expected, ok := want[mod.StatID]; ok {
			if mod.Text != expected.text || !slices.Equal(mod.Values, []float64{expected.value}) {
				t.Fatalf("%s: %+v, want %+v", mod.StatID, mod, expected)
			}
			if mod.StatID == "explicit.spell" && (!mod.Selected || mod.Tier != 1 || mod.Affix != "suffix") {
				t.Fatalf("spell search metadata changed: %+v", mod)
			}
			delete(want, mod.StatID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing matched stats: %v", want)
	}
}

func TestClipboardQualityWithoutMagnitudeIsNotAppliedAgain(t *testing.T) {
	raw := strings.ReplaceAll(catalystClipboardItem, " — 40% Increased", "")
	raw = strings.ReplaceAll(raw, "+3 to Level of all Spell Skills", "+4 to Level of all Spell Skills")
	raw = strings.ReplaceAll(raw, "26(25-28)% increased Cast Speed", "36% increased Cast Speed")
	item, err := ParseItem(raw, catalystClipboardCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, mod := range item.Mods {
		if mod.StatID == "explicit.spell" && !slices.Equal(mod.Values, []float64{4}) ||
			mod.StatID == "explicit.speed" && !slices.Equal(mod.Values, []float64{36}) {
			t.Fatalf("already augmented stat scaled again: %+v", mod)
		}
	}
}

func TestClipboardMagnitudeUsesHeaderRatherThanQualityOrSanctifiedFlag(t *testing.T) {
	for _, tc := range []struct {
		name, header, base, state string
		want                      float64
	}{
		{"below_threshold", "33", "3", "", 3},
		{"threshold", "34", "3", "", 4},
		{"zero", "0", "3", "", 3},
		{"sanctified_base_four", "40", "4", "\nSanctified", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.ReplaceAll(catalystClipboardItem, "40% Increased", tc.header+"% Increased")
			raw = strings.ReplaceAll(raw, "+3 to Level of all Spell Skills", "+"+tc.base+" to Level of all Spell Skills") + tc.state
			item, err := ParseItem(raw, catalystClipboardCatalog())
			if err != nil {
				t.Fatal(err)
			}
			for _, mod := range item.Mods {
				if mod.StatID == "explicit.spell" {
					if !slices.Equal(mod.Values, []float64{tc.want}) {
						t.Fatalf("spell values=%v, want %v", mod.Values, tc.want)
					}
					return
				}
			}
			t.Fatal("spell stat missing")
		})
	}
}

func TestClipboardMagnitudePrecedesMergeAndPseudoTotals(t *testing.T) {
	raw := `Item Class: Rings
Rarity: Rare
Test Ring
Gold Ring
--------
Quality (Life Modifiers): +40% (augmented)
--------
{ Prefix Modifier "Test" (Tier: 1) — Life — 40% Increased }
+50(40-50) to maximum Life
+10 to maximum Mana — Unscalable Value
{ Prefix Modifier "Another" (Tier: 2) — Life — 20% Increased }
+50(40-50) to maximum Life
{ Suffix Modifier "Plain" (Tier: 1) }
+10 to maximum Mana`
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.mana", Text: "# to maximum Mana", Type: "explicit"})
	item, err := ParseItem(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	foundLife, foundMana, foundTotal := false, false, false
	for _, mod := range item.Mods {
		switch mod.StatID {
		case "explicit.stat_3299347043":
			foundLife = slices.Equal(mod.Values, []float64{130}) && mod.Text == "+130 to maximum Life"
		case "explicit.mana":
			foundMana = slices.Equal(mod.Values, []float64{20})
		}
		if mod.Type == "pseudo" && strings.Contains(mod.Text, "total maximum Life") {
			foundTotal = slices.Equal(mod.Values, []float64{130})
		}
	}
	if !foundLife || !foundMana || !foundTotal {
		t.Fatalf("scaled merge/pseudo or unscalable/reset handling failed: %+v", item.Mods)
	}
}

func BenchmarkParseCatalystClipboard(b *testing.B) {
	cat := catalystClipboardCatalog()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseItem(catalystClipboardItem, cat); err != nil {
			b.Fatal(err)
		}
	}
}
