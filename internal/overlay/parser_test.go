package overlay

import (
	"strings"
	"testing"
)

func testCatalog() Catalog {
	return Catalog{Stats: []StatGroup{
		{ID: "explicit", Entries: []StatEntry{
			{ID: "explicit.stat_3299347043", Text: "# to maximum Life", Type: "explicit"},
			{ID: "explicit.stat_4015621042", Text: "#% increased Energy Shield", Type: "explicit"},
			{ID: "explicit.stat_3874491706", Text: "All Mage's Legacies have #% increased effect per duplicate Mage's Legacy you have", Type: "explicit"},
			{ID: "explicit.stat_264262054|5", Text: "Legacy of Gold", Type: "explicit"},
		}},
		{ID: "implicit", Entries: []StatEntry{
			{ID: "implicit.stat_1416292992", Text: "Has # Charm Slot", Type: "implicit"},
			{ID: "implicit.stat_462041840", Text: "#% of Flask Recovery applied Instantly", Type: "implicit"},
		}},
	}}
}

func TestParseUniqueAdvancedItem(t *testing.T) {
	raw := `Item Class: Belts
Rarity: Unique
Mageblood
Utility Belt
------------
## Requires: Level 55
## Item Level: 80
{ Implicit Modifier }
20% of Flask Recovery applied Instantly
{ Implicit Modifier — Charm }
Has 3(1-3) Charm Slots
----------------------
{ Unique Modifier }
Legacy of Gold(Amethyst-Topaz) — Unscalable Value
{ Unique Modifier }
All Mage's Legacies have 28(25-50)% increased effect per duplicate Mage's Legacy you have
-----------------------------------------------------------------------------------------`
	item, err := ParseItem(raw, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "Mageblood" || item.BaseType != "Utility Belt" || item.ItemLevel != 80 || item.RequiredLevel != 55 {
		t.Fatalf("wrong item: %+v", item)
	}
	if len(item.Mods) != 4 {
		t.Fatalf("mods=%d, want 4: %+v", len(item.Mods), item.Mods)
	}
	for _, mod := range item.Mods {
		if mod.StatID == "" || !mod.Selected {
			t.Fatalf("unmatched mod: %+v", mod)
		}
	}
	if got := item.Mods[1].Values; len(got) != 1 || got[0] != 3 {
		t.Fatalf("charm values=%v", got)
	}
}

func TestParseRareAffixes(t *testing.T) {
	raw := `Item Class: Helmets
Rarity: Rare
Rapture Salvation
Kamasan Tiara
-------------
Quality (Defence Modifiers): +40% (augmented)
Energy Shield: 503 (augmented)
## Requires: Level 75, 103 (augmented) Int
## Item Level: 81
{ Prefix Modifier "Pope's" (Tier: 1) — Life, Energy Shield }
41(39-42)% increased Energy Shield
+44(42-49) to maximum Life
{ Suffix Modifier "of the Ice" (Tier: 2) — Elemental, Cold, Resistance }
+39(36-40)% to Cold Resistance`
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.hybrid", Text: "#% increased Energy Shield\n# to maximum Life", Type: "explicit"},
		StatEntry{ID: "explicit.cold", Text: "#% to Cold Resistance", Type: "explicit"},
	)
	item, err := ParseItem(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "Rapture Salvation" || item.BaseType != "Kamasan Tiara" || item.Quality != 40 {
		t.Fatalf("wrong item: %+v", item)
	}
	if len(item.Properties) == 0 || item.Properties[0].Name != "Quality (Defence Modifiers)" {
		t.Fatalf("quality property label was not preserved: %+v", item.Properties)
	}
	if len(item.Mods) != 2 || item.Mods[0].Affix != "prefix" || item.Mods[0].Tier != 1 || item.Mods[1].Affix != "suffix" {
		t.Fatalf("wrong mods: %+v", item.Mods)
	}
	if item.Mods[0].StatID != "explicit.hybrid" || len(item.Mods[0].Values) != 2 {
		t.Fatalf("hybrid mismatch: %+v", item.Mods[0])
	}
}

func TestParseItemStates(t *testing.T) {
	raw := `Item Class: Amulets
Rarity: Rare
Sorrow Noose
Absent Amulet
-------------
## Item Level: 79
{ Prefix Modifier "Countess'" (Tier: 1) }
+50(47-50) to Spirit
----------------------
Corrupted
## Sanctified
Fractured Item`
	item, err := ParseItem(raw, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if !item.Fractured || !item.Corrupted || !item.Sanctified {
		t.Fatalf("item states were not parsed: fractured=%v corrupted=%v sanctified=%v", item.Fractured, item.Corrupted, item.Sanctified)
	}
}

func TestParseFracturedAffix(t *testing.T) {
	raw := `Item Class: Amulets
Rarity: Rare
Sorrow Noose
Absent Amulet
-------------
{ Fractured Prefix Modifier "Countess'" (Tier: 1) }
+50(47-50) to Spirit
{ Crafted Prefix Modifier "Essences" }
28(20-30)% increased Global Armour, Evasion and Energy Shield
{ Desecrated Suffix Modifier "of Kurgal" (Tier: 1) }
+6(3-5)% to Quality of all Skills
---------------------------------
Fractured Item`
	cat := testCatalog()
	cat.Stats = append(cat.Stats, StatGroup{ID: "fractured", Entries: []StatEntry{
		{ID: "fractured.spirit", Text: "# to Spirit", Type: "fractured"},
	}})
	cat.Stats = append(cat.Stats, StatGroup{ID: "crafted", Entries: []StatEntry{
		{ID: "crafted.defence", Text: "#% increased Global Armour, Evasion and Energy Shield", Type: "crafted"},
	}})
	cat.Stats = append(cat.Stats, StatGroup{ID: "desecrated", Entries: []StatEntry{
		{ID: "desecrated.quality", Text: "#% to Quality of all Skills", Type: "desecrated"},
	}})
	item, err := ParseItem(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !item.Fractured || len(item.Mods) != 3 {
		t.Fatalf("fractured item was not parsed: %+v", item)
	}
	if item.Mods[0].Type != "fractured" || item.Mods[0].Affix != "prefix" || item.Mods[0].StatID != "fractured.spirit" {
		t.Fatalf("fractured affix was flattened: %+v", item.Mods[0])
	}
	if item.Mods[1].Type != "crafted" || item.Mods[2].Type != "desecrated" {
		t.Fatalf("special affix types were flattened: %+v", item.Mods)
	}
}

func TestParseUnidentifiedUniqueUsesBaseTypeWithoutName(t *testing.T) {
	raw := `Item Class: Rings
Rarity: Unique
Pearl Ring
----------
## Item Level: 79
{ Implicit Modifier — Caster, Speed }
9(7-10)% increased Cast Speed
-----------------------------
## Unidentified`
	cat := testCatalog()
	cat.Stats[1].Entries = append(cat.Stats[1].Entries,
		StatEntry{ID: "implicit.cast_speed", Text: "#% increased Cast Speed", Type: "implicit"},
	)
	item, err := ParseItem(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !item.Unidentified || item.Name != "" || item.BaseType != "Pearl Ring" {
		t.Fatalf("unidentified unique title was parsed as a named unique: %+v", item)
	}
	if len(item.Mods) != 1 || item.Mods[0].StatID != "implicit.cast_speed" {
		t.Fatalf("implicit mod was not preserved: %+v", item.Mods)
	}
}

func TestParseCorruptionEnchantAndRunes(t *testing.T) {
	raw := `Item Class: Sceptres
Rarity: Unique
Sacred Flame
Shrine Sceptre
--------
Spirit: 115 (augmented)
--------
Item Level: 84
--------
{ Corruption Enhancement }
15(15-25)% increased Spirit
--------
40% reduced Presence Area of Effect (rune)
--------
{ Unique Modifier — Damage, Elemental, Fire }
Gain 59(40-60)% of Damage as Extra Fire Damage
--------
Twice Corrupted`
	catalog := Catalog{Stats: []StatGroup{
		{ID: "explicit", Entries: []StatEntry{
			{ID: "explicit.stat_3984865854", Text: "#% increased Spirit", Type: "explicit"},
			{ID: "explicit.stat_fire", Text: "Gain #% of Damage as Extra Fire Damage", Type: "explicit"},
		}},
		{ID: "enchant", Entries: []StatEntry{
			{ID: "enchant.stat_3984865854", Text: "#% increased Spirit", Type: "enchant"},
		}},
		{ID: "rune", Entries: []StatEntry{
			{ID: "rune.stat_presence", Text: "#% reduced Presence Area of Effect", Type: "augment"},
		}},
	}}
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !item.TwiceCorrupted || item.Corrupted {
		t.Fatalf("Twice Corrupted must be its own state: twice=%v corrupted=%v", item.TwiceCorrupted, item.Corrupted)
	}
	if len(item.Mods) != 3 {
		t.Fatalf("mods=%d, want 3: %+v", len(item.Mods), item.Mods)
	}
	enchant, rune := item.Mods[0], item.Mods[1]
	if enchant.Type != "enchant" || enchant.StatID != "enchant.stat_3984865854" || !enchant.Selected || len(enchant.Values) != 1 || enchant.Values[0] != 15 {
		t.Fatalf("corruption enchant: %+v", enchant)
	}
	if rune.Type != "rune" || rune.StatID != "rune.stat_presence" || rune.Selected {
		t.Fatalf("rune: %+v", rune)
	}
}

func TestParseSkipsUsabilityWarningInTitle(t *testing.T) {
	raw := `Item Class: Sceptres
Rarity: Unique
You cannot use this item. Its stats will be ignored
--------
Sacred Flame
Shrine Sceptre
--------
Item Level: 84`
	item, err := ParseItem(raw, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "Sacred Flame" || item.BaseType != "Shrine Sceptre" {
		t.Fatalf("name=%q base=%q", item.Name, item.BaseType)
	}
}

func TestParseGrantedSkillAndAllocatesEnchant(t *testing.T) {
	raw := `Item Class: Amulets
Rarity: Rare
Eagle Choker
Absent Amulet
--------
Item Level: 80
--------
{ Enhancement }
Allocates Thaumaturgic Generator — Unscalable Value
--------
{ Implicit Modifier }
-1 Prefix Modifier allowed
--------
Grants Skill: Level 14 Eternal Rage (Max Level 18)
--------
{ Suffix Modifier "of the Sharpshooter" (Tier: 1) }
+3 to Level of all Projectile Skills
--------
Fractured Item`
	catalog := Catalog{Stats: []StatGroup{
		{ID: "enchant", Entries: []StatEntry{
			{ID: "enchant.stat_2954116742|56666", Text: "Allocates Thaumaturgic Generator", Type: "enchant"},
		}},
		{ID: "skill", Entries: []StatEntry{
			{ID: "skill.eternal_rage", Text: "Grants Skill: Level # Eternal Rage", Type: "skill"},
		}},
	}}
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string]ItemMod{}
	for _, mod := range item.Mods {
		byType[mod.Type] = mod
	}
	if m := byType["enchant"]; m.StatID != "enchant.stat_2954116742|56666" || !m.Selected {
		t.Fatalf("allocates enchant: %+v", m)
	}
	if m := byType["skill"]; m.StatID != "skill.eternal_rage" || !m.Selected || len(m.Values) != 1 || m.Values[0] != 14 {
		t.Fatalf("granted skill: %+v", m)
	}
}

func TestParseRuneSocketsAndWard(t *testing.T) {
	raw := `Item Class: Gloves
Rarity: Rare
Beast Hold
Runeforged Secured Wraps
--------
Evasion Rating: 41 (augmented)
Runic Ward: 166
--------
Sockets: S S 
--------
Item Level: 80`
	item, err := ParseItem(raw, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if item.RuneSockets != 2 {
		t.Fatalf("rune sockets=%d, want 2", item.RuneSockets)
	}
	var ward bool
	for _, p := range item.Properties {
		ward = ward || (p.Name == "Runic Ward" && p.Value == "166")
	}
	if !ward {
		t.Fatalf("runic ward missing: %+v", item.Properties)
	}
}

// The expected numbers are what PoE2 Overlay and the trade site show for the
// same two spears.
func TestParseWeaponDPSAndMirrored(t *testing.T) {
	cases := []struct {
		raw             string
		pdps, edps, dps string
		mirrored, twice bool
	}{
		{raw: `Item Class: Spears
Rarity: Rare
Corruption Edge
Flying Spear
--------
Quality: +30% (augmented)
Physical Damage: 308-519 (augmented)
Lightning Damage: 4-233 (lightning)
Critical Hit Chance: 10.00% (augmented)
Attacks per Second: 1.60
--------
Item Level: 82
--------
{ Prefix Modifier "Flaring" (Tier: 1) — Damage, Physical, Attack }
Adds 39(26-39) to 66(44-66) Physical Damage
--------
Mirrored
--------
Fractured Item`, pdps: "661.6", edps: "189.6", dps: "851.2", mirrored: true},
		{raw: `Item Class: Spears
Rarity: Rare
You cannot use this item. Its stats will be ignored
--------
Miracle Edge
Soaring Spear
--------
Quality: +20% (augmented)
Physical Damage: 58-107 (augmented)
Lightning Damage: 1-43 (lightning)
Critical Hit Chance: 5.00%
Attacks per Second: 2.69 (augmented)
--------
Item Level: 81
--------
Twice Corrupted`, pdps: "221.93", edps: "59.18", dps: "281.11", twice: true},
	}
	for _, c := range cases {
		item, err := ParseItem(c.raw, Catalog{})
		if err != nil {
			t.Fatal(err)
		}
		props := map[string]string{}
		for _, p := range item.Properties {
			props[p.Name] = p.Value
		}
		if props["Physical DPS"] != c.pdps || props["Elemental DPS"] != c.edps || props["DPS"] != c.dps {
			t.Errorf("%s: pdps=%q edps=%q dps=%q, want %s/%s/%s", item.BaseType, props["Physical DPS"], props["Elemental DPS"], props["DPS"], c.pdps, c.edps, c.dps)
		}
		if item.Mirrored != c.mirrored || item.TwiceCorrupted != c.twice {
			t.Errorf("%s: mirrored=%v twice=%v", item.BaseType, item.Mirrored, item.TwiceCorrupted)
		}
		for _, mod := range item.Mods {
			if strings.Contains(mod.Text, "Mirrored") {
				t.Errorf("Mirrored leaked into a modifier: %+v", mod)
			}
		}
	}
}

func TestParseSplitsHybridModifierButKeepsMultiLineStat(t *testing.T) {
	raw := `Item Class: Helmets
Rarity: Rare
Rapture Salvation
Kamasan Tiara
--------
Item Level: 81
--------
{ Prefix Modifier "Pope's" (Tier: 1) — Life, Energy Shield }
41(39-42)% increased Energy Shield
+44(42-49) to maximum Life
{ Unique Modifier }
Spells fire 2 additional Projectiles
Spells fire Projectiles in a circle`
	catalog := Catalog{Stats: []StatGroup{{ID: "explicit", Entries: []StatEntry{
		{ID: "explicit.es", Text: "#% increased Energy Shield", Type: "explicit"},
		{ID: "explicit.life", Text: "# to maximum Life", Type: "explicit"},
		{ID: "explicit.circle", Text: "Spells fire # additional Projectiles\nSpells fire Projectiles in a circle", Type: "explicit"},
	}}}}
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range item.Mods {
		got = append(got, m.StatID)
	}
	want := []string{"explicit.es", "explicit.life", "explicit.circle"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stats=%v, want %v", got, want)
	}
	if item.Mods[1].Values[0] != 44 || item.Mods[1].Tier != 1 || item.Mods[1].Name != "Pope's" {
		t.Fatalf("life half lost its values or header: %+v", item.Mods[1])
	}
}

func TestMatchesCatalogStatWithSignOutsidePlaceholder(t *testing.T) {
	raw := `Item Class: Crossbows
Rarity: Rare
Siege Crossbow
--------
Item Level: 80
--------
{ Desecrated Prefix Modifier "Amanamu's" (Tier: 1) }
Grenade Skills have +1 Cooldown Use`
	catalog := Catalog{Stats: []StatGroup{{ID: "desecrated", Entries: []StatEntry{
		{ID: "desecrated.stat_2250681686", Text: "Grenade Skills have +# Cooldown Use", Type: "desecrated"},
	}}}}
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Mods) != 1 || item.Mods[0].StatID != "desecrated.stat_2250681686" || !item.Mods[0].Selected {
		t.Fatalf("mods=%+v", item.Mods)
	}
}
