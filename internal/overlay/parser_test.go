package overlay

import (
	"fmt"
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
	item.Mods = ownMods(item.Mods)
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
	item.Mods = ownMods(item.Mods)
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
	item.Mods = ownMods(item.Mods)
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
	item.Mods = ownMods(item.Mods)
	if len(item.Mods) != 1 || item.Mods[0].StatID != "desecrated.stat_2250681686" || !item.Mods[0].Selected {
		t.Fatalf("mods=%+v", item.Mods)
	}
}

func TestListingStatIDUsesHashesForSameWording(t *testing.T) {
	catalog := Catalog{Stats: []StatGroup{{Entries: []StatEntry{
		{ID: "explicit.stat_1", Text: "#% increased Physical Damage", Type: "explicit"},
		{ID: "explicit.stat_2", Text: "#% increased Physical Damage (Local)", Type: "explicit"},
		{ID: "explicit.stat_3", Text: "# to maximum Mana", Type: "explicit"},
	}}}}
	if got := ListingStatID("+250 to maximum Mana", "explicit", nil, catalog); got != "explicit.stat_3" {
		t.Fatalf("mana = %q", got)
	}
	if got := ListingStatID("150% increased Physical Damage", "explicit", []string{"stat_2"}, catalog); got != "explicit.stat_2" {
		t.Fatalf("local physical = %q", got)
	}
	if got := ListingStatID("150% increased Physical Damage", "explicit", nil, catalog); got != "explicit.stat_1" {
		t.Fatalf("no hash falls back to first = %q", got)
	}
	if got := ListingStatID("Something unknown", "explicit", nil, catalog); got != "" {
		t.Fatalf("unknown = %q", got)
	}
}

func emptyAffixes(item Item) map[string]float64 {
	out := map[string]float64{}
	for _, mod := range item.Mods {
		if mod.Type == "pseudo" {
			out[mod.StatID] = mod.Values[0]
			if mod.Selected {
				out["selected"] = 1
			}
		}
	}
	return out
}

func TestEmptyAffixSlotsCountHeadersNotLines(t *testing.T) {
	// Two prefixes and two suffixes: one of each is open.
	boots := "Item Class: Boots\nRarity: Rare\nRapture Hoof\nSerpentscale Boots\n--------\nItem Level: 82\n--------\n" +
		"{ Prefix Modifier \"Vaporous\" (Tier: 1) — Evasion }\n+162(147-176) to Evasion Rating\n" +
		"{ Prefix Modifier \"Mirage's\" (Tier: 1) — Evasion }\n100(92-100)% increased Evasion Rating\n" +
		"{ Suffix Modifier \"of Recuperation\" (Tier: 1) — Life }\n22(18.1-23) Life Regeneration per second\n" +
		"{ Suffix Modifier \"of Diversion\" (Tier: 2) — Evasion }\nGain Deflection Rating equal to 18(18-20)% of Evasion Rating\n"
	item, err := ParseItem(boots, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	got := emptyAffixes(item)
	if got[emptyPrefixStat] != 1 || got[emptySuffixStat] != 1 || got["selected"] != 0 {
		t.Fatalf("boots empty affixes = %v", got)
	}

	// Seven stat lines, but three prefix and three suffix headers (a hybrid,
	// a desecrated two-line prefix, a fractured suffix): nothing is open.
	shoes := "Item Class: Boots\nRarity: Rare\nAnarchy Dash\nCharmed Shoes\n--------\nItem Level: 79\n--------\n" +
		"20% increased Armour, Evasion and Energy Shield (rune)\n--------\n" +
		"{ Prefix Modifier \"Illusory\" (Tier: 1) — Evasion, Energy Shield }\n96(92-100)% increased Evasion and Energy Shield\n" +
		"{ Prefix Modifier \"Trickster's\" (Tier: 1) }\n41(39-42)% increased Evasion and Energy Shield\n+126(95-136) to Stun Threshold\n" +
		"{ Desecrated Prefix Modifier \"Cherub's\" (Tier: 1) — Evasion, Energy Shield }\n+70(65-78) to Evasion Rating\n+24(22-25) to maximum Energy Shield\n" +
		"{ Fractured Suffix Modifier \"of Chronomancy\" (Tier: 1) }\nSkills have 15(11-15)% chance to not remove Charges but still count as consuming them\n" +
		"{ Suffix Modifier \"of Chronomancy\" (Tier: 1) }\n36(30-40)% increased Skill Effect Duration\n" +
		"{ Suffix Modifier \"of Flexure\" (Tier: 1) — Evasion }\nGain Deflection Rating equal to 23(21-23)% of Evasion Rating\n" +
		"--------\nCorrupted\n--------\nFractured Item\n"
	item, err = ParseItem(shoes, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if got := emptyAffixes(item); len(got) != 0 {
		t.Fatalf("shoes empty affixes = %v", got)
	}

	// A plain copy has no headers, so the slots are unknown.
	plain := "Item Class: Boots\nRarity: Rare\nRapture Hoof\nSerpentscale Boots\n--------\nItem Level: 82\n--------\n+162 to Evasion Rating\n"
	item, err = ParseItem(plain, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if got := emptyAffixes(item); len(got) != 0 {
		t.Fatalf("plain copy empty affixes = %v", got)
	}
}

// ownMods drops the empty-slot pseudo stats, for tests about the item's own
// modifier lines.
func ownMods(mods []ItemMod) []ItemMod {
	out := []ItemMod{}
	for _, mod := range mods {
		if mod.Type != "pseudo" {
			out = append(out, mod)
		}
	}
	return out
}

func TestJewelAffixSlotsAreTwoPerSide(t *testing.T) {
	jewel := "Item Class: Jewels\nRarity: Rare\nViper Ichor\nEmerald\n--------\nItem Level: 80\n--------\n" +
		"{ Prefix Modifier \"Honed\" (Tier: 1) }\n15(5-15)% increased Projectile Damage\n" +
		"{ Suffix Modifier \"of the Spear\" (Tier: 1) }\n17(10-20)% increased Critical Damage Bonus with Spears\n"
	item, err := ParseItem(jewel, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	got := emptyAffixes(item)
	if got[emptyPrefixStat] != 1 || got[emptySuffixStat] != 1 {
		t.Fatalf("jewel empty affixes = %v", got)
	}
	// Past the usual count (a special craft or corruption): full, not negative.
	full := jewel + "{ Suffix Modifier \"of Rupture\" (Tier: 1) }\n9(6-16)% increased Critical Hit Chance for Attacks\n" +
		"{ Suffix Modifier \"of Unmaking\" (Tier: 1) }\n20(10-20)% increased Critical Damage Bonus for Attack Damage\n"
	item, err = ParseItem(full, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if got := emptyAffixes(item); got[emptySuffixStat] != 0 || got[emptyPrefixStat] != 1 {
		t.Fatalf("overfull jewel empty affixes = %v", got)
	}
}

func TestSameStatAffixesAreSummedLikeTheTradeIndex(t *testing.T) {
	catalog := Catalog{Stats: []StatGroup{{Entries: []StatEntry{
		{ID: "explicit.stat_1999113824", Text: "#% increased Evasion and Energy Shield", Type: "explicit"},
		{ID: "explicit.stat_915769802", Text: "# to Stun Threshold", Type: "explicit"},
	}}}}
	raw := "Item Class: Boots\nRarity: Rare\nAnarchy Dash\nCharmed Shoes\n--------\nItem Level: 79\n--------\n" +
		"{ Prefix Modifier \"Illusory\" (Tier: 1) — Evasion, Energy Shield }\n96(92-100)% increased Evasion and Energy Shield\n" +
		"{ Prefix Modifier \"Trickster's\" (Tier: 2) }\n41(39-42)% increased Evasion and Energy Shield\n+126(95-136) to Stun Threshold\n"
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	mods := ownMods(item.Mods)
	if len(mods) != 2 {
		t.Fatalf("mods = %+v", mods)
	}
	m := mods[0]
	if m.StatID != "explicit.stat_1999113824" || m.Values[0] != 137 || m.Text != "137% increased Evasion and Energy Shield" ||
		fmt.Sprint(m.Tiers) != "[1 2]" || m.Name != "Illusory + Trickster's" || !m.Selected {
		t.Fatalf("merged = %+v", m)
	}
	if mods[1].Text != "+126(95-136) to Stun Threshold" || len(mods[1].Tiers) != 0 {
		t.Fatalf("stun = %+v", mods[1])
	}
	// Both prefixes are still counted as two slots.
	if got := emptyAffixes(item); got[emptyPrefixStat] != 1 {
		t.Fatalf("empty = %v", got)
	}
}

func TestWaystoneIsSearchedByItsTotals(t *testing.T) {
	raw := "Item Class: Waystones\nRarity: Rare\nForsaken Path\nWaystone (Tier 15)\n--------\n" +
		"Revives Available: 0 (augmented)\nItem Rarity: +26% (augmented)\nPack Size: +9% (augmented)\nMonster Rarity: +60% (augmented)\n" +
		"Monster Effectiveness: +15% (augmented)\nWaystone Drop Chance: +90% (augmented)\n--------\nItem Level: 81\n--------\n" +
		"{ Prefix Modifier \"Tough\" (Tier: 1) }\n21(20-25)% more Monster Life\n" +
		"--------\nCan be used in a Map Device, allowing you to enter a Map. Waystones can only be used once.\n--------\nCorrupted\n"
	catalog := Catalog{Stats: []StatGroup{{Entries: []StatEntry{{ID: "explicit.stat_95249895", Text: "#% more Monster Life", Type: "explicit"}}}}}
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range item.Properties {
		got[p.Name] = p.Value
	}
	want := map[string]string{"Revives Available": "0", "Item Rarity": "+26%", "Pack Size": "+9%", "Monster Rarity": "+60%", "Monster Effectiveness": "+15%", "Waystone Drop Chance": "+90%"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("properties = %v", got)
	}
	if item.BaseType != "Waystone (Tier 15)" || !item.Corrupted {
		t.Fatalf("item = %+v", item)
	}
	for _, mod := range item.Mods {
		if mod.Selected || mod.Type == "pseudo" {
			t.Fatalf("waystone mod should be optional and have no empty slots: %+v", mod)
		}
	}
	if len(item.Mods) != 1 || item.Mods[0].StatID == "" {
		t.Fatalf("mods = %+v", item.Mods)
	}
}

func TestWeaponsAndArmourMatchTheLocalStat(t *testing.T) {
	catalog := Catalog{Stats: []StatGroup{{Entries: []StatEntry{
		{ID: "explicit.stat_2866361420", Text: "#% increased Armour", Type: "explicit"},
		{ID: "explicit.stat_1062208444", Text: "#% increased Armour (Local)", Type: "explicit"},
		{ID: "explicit.stat_774059442", Text: "# to maximum Runic Ward", Type: "explicit"},
		{ID: "explicit.stat_3336230913", Text: "# to maximum Runic Ward", Type: "explicit"},
	}}}}
	shield := "Item Class: Shields\nRarity: Unique\nSvalinn\nRunemastered Crucible Tower Shield\n--------\nItem Level: 82\n--------\n" +
		"{ Unique Modifier — Armour }\n324(200-300)% increased Armour\n{ Unique Modifier }\n+96(50-100) to maximum Runic Ward\n"
	item, err := ParseItem(shield, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if item.Mods[0].StatID != "explicit.stat_1062208444" || item.Mods[1].StatID != "explicit.stat_774059442" {
		t.Fatalf("shield mods = %+v", item.Mods)
	}
	// The same wording on jewellery is the global stat.
	ring := "Item Class: Rings\nRarity: Rare\nDoom Loop\nIron Ring\n--------\nItem Level: 82\n--------\n" +
		"{ Prefix Modifier \"Plated\" (Tier: 1) }\n30% increased Armour\n"
	item, err = ParseItem(ring, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if item.Mods[0].StatID != "explicit.stat_2866361420" {
		t.Fatalf("ring mod = %+v", item.Mods[0])
	}
}

func TestAmbiguousStatsKeepTheirTwins(t *testing.T) {
	catalog := Catalog{Stats: []StatGroup{{Entries: []StatEntry{
		{ID: "explicit.stat_2866361420", Text: "#% increased Armour", Type: "explicit"},
		{ID: "explicit.stat_1062208444", Text: "#% increased Armour (Local)", Type: "explicit"},
		{ID: "explicit.stat_774059442", Text: "# to maximum Runic Ward", Type: "explicit"},
		{ID: "explicit.stat_3336230913", Text: "# to maximum Runic Ward", Type: "explicit"},
		{ID: "explicit.stat_1", Text: "Chance to Block Damage is Lucky", Type: "explicit"},
	}}}}
	raw := "Item Class: Shields\nRarity: Unique\nSvalinn\nCrucible Tower Shield\n--------\nItem Level: 82\n--------\n" +
		"{ Unique Modifier }\n324(200-300)% increased Armour\n{ Unique Modifier }\n+96(50-100) to maximum Runic Ward\n{ Unique Modifier }\nChance to Block Damage is Lucky\n"
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, m := range item.Mods {
		got = append(got, m.StatID+"+"+strings.Join(m.AltStatIDs, ","))
	}
	want := "explicit.stat_1062208444+explicit.stat_2866361420 explicit.stat_774059442+explicit.stat_3336230913 explicit.stat_1+"
	if strings.Join(got, " ") != want {
		t.Fatalf("stats = %v", got)
	}
}

func TestRequiredLevelIgnoresAttributes(t *testing.T) {
	for raw, want := range map[string]int{
		"Requires: 30 (augmented) Str, 30 (augmented) Dex": 0,
		"Requires: Level 65, 87 (augmented) Str":           65,
		"Requires: Level 44, 52 Dex":                       44,
	} {
		item, err := ParseItem("Item Class: Body Armours\nRarity: Unique\nExplorer Armour\n--------\n"+raw+"\n--------\nItem Level: 82\n--------\nUnidentified\n", Catalog{})
		if err != nil {
			t.Fatal(err)
		}
		if item.RequiredLevel != want {
			t.Errorf("%q: required level %d, want %d", raw, item.RequiredLevel, want)
		}
	}
}
