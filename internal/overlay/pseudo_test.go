package overlay

import "testing"

func pseudoByID(item Item) map[string]ItemMod {
	out := map[string]ItemMod{}
	for _, mod := range item.Mods {
		if mod.Type == "pseudo" {
			out[mod.StatID] = mod
		}
	}
	return out
}

func modByText(t *testing.T, item Item, text string) ItemMod {
	t.Helper()
	for _, mod := range item.Mods {
		if mod.Type != "pseudo" && mod.Text == text {
			return mod
		}
	}
	t.Fatalf("no mod %q in %+v", text, item.Mods)
	return ItemMod{}
}

// The same item POE2 Overlay prices by 29% fire, 14% cold, 43% elemental and
// 12 mana: the rune's fire resistance counts toward the fire total.
func TestPseudoTotalsOfARare(t *testing.T) {
	raw := `Item Class: Gloves
Rarity: Rare
Grim Grasp
Ringmail Gauntlets
--------
Armour: 16
Evasion Rating: 13
--------
Requires: Level 30
--------
Sockets: S
--------
Item Level: 24
--------
+18% to Fire Resistance (rune)
--------
{ Prefix Modifier "Frozen" (Tier: 7) — Damage, Elemental, Cold, Attack }
Adds 5(4-6) to 11(8-11) Cold damage to Attacks
{ Prefix Modifier "Humming" (Tier: 7) — Damage, Elemental, Lightning, Attack }
Adds 1 to 19(18-20) Lightning damage to Attacks
{ Prefix Modifier "Azure" (Tier: 9) — Mana }
+12(10-14) to maximum Mana
{ Suffix Modifier "of Absorption" (Tier: 6) — Mana }
Gain 6 Mana per enemy killed
{ Suffix Modifier "of the Seal" (Tier: 7) — Elemental, Cold, Resistance }
+14(11-15)% to Cold Resistance
{ Suffix Modifier "of the Whelpling" (Tier: 7) — Elemental, Fire, Resistance }
+11(11-15)% to Fire Resistance`
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.mana", Text: "# to maximum Mana", Type: "explicit"},
		StatEntry{ID: "explicit.cold", Text: "#% to Cold Resistance", Type: "explicit"},
		StatEntry{ID: "explicit.fire", Text: "#% to Fire Resistance", Type: "explicit"},
		StatEntry{ID: "explicit.adds", Text: "Adds # to # Cold damage to Attacks", Type: "explicit"},
	)
	item, err := ParseItem(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	got := pseudoByID(item)
	// Each element's total is folded away beside the elemental total, which
	// already asks for the same resistances.
	want := map[string]struct {
		value float64
		shown bool
	}{
		"pseudo.pseudo_total_mana":                 {12, true}, // no Intelligence on the item
		"pseudo.pseudo_total_fire_resistance":      {29, false},
		"pseudo.pseudo_total_cold_resistance":      {14, false},
		"pseudo.pseudo_total_elemental_resistance": {43, true},
	}
	for id, w := range want {
		mod, ok := got[id]
		if !ok || len(mod.Values) != 1 || mod.Values[0] != w.value || mod.Selected != w.shown || mod.Hidden == w.shown {
			t.Errorf("%s = %+v, want %v selected and shown: %v", id, mod, w.value, w.shown)
		}
	}
	if _, ok := got["pseudo.pseudo_total_life"]; ok {
		t.Error("no life on the item, yet a life total")
	}
	// The lines the totals add up leave the search; the rest stay.
	for _, text := range []string{"+12(10-14) to maximum Mana", "+14(11-15)% to Cold Resistance", "+11(11-15)% to Fire Resistance"} {
		if mod := modByText(t, item, text); mod.Selected || !mod.Hidden {
			t.Errorf("%q is in a total but still selected or shown", text)
		}
	}
	if !modByText(t, item, "Adds 5(4-6) to 11(8-11) Cold damage to Attacks").Selected {
		t.Error("a line no total covers was unselected")
	}
}

// The two "Adds # to # damage to Attacks" lines average 8 and 10: POE2
// Overlay searches them as a Weighted Sum v2 group, min 18. GGG refuses such
// groups without a login, so the sum starts selected only when signed in.
func TestElementalAttackDamageSum(t *testing.T) {
	raw := `Item Class: Gloves
Rarity: Rare
Grim Grasp
Ringmail Gauntlets
--------
Item Level: 24
--------
{ Prefix Modifier "Frozen" (Tier: 7) — Damage, Elemental, Cold, Attack }
Adds 5(4-6) to 11(8-11) Cold damage to Attacks
{ Prefix Modifier "Humming" (Tier: 7) — Damage, Elemental, Lightning, Attack }
Adds 1 to 19(18-20) Lightning damage to Attacks`
	cold, lightning := "Adds 5(4-6) to 11(8-11) Cold damage to Attacks", "Adds 1 to 19(18-20) Lightning damage to Attacks"
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.stat_4067062424", Text: "Adds # to # Cold damage to Attacks", Type: "explicit"},
		StatEntry{ID: "explicit.stat_1754445556", Text: "Adds # to # Lightning damage to Attacks", Type: "explicit"},
	)
	for _, signedIn := range []bool{false, true} {
		item, err := ParseItemWith(raw, cat, ParseOptions{SignedIn: signedIn})
		if err != nil {
			t.Fatal(err)
		}
		sum, ok := pseudoByID(item)[elementalAttackSumID]
		if !ok || sum.Values[0] != 18 || len(sum.WeightStats) == 0 {
			t.Fatalf("sum = %+v, want 18 with weighted stats", sum)
		}
		if sum.Selected != signedIn {
			t.Errorf("signed in %v: sum selected %v", signedIn, sum.Selected)
		}
		if modByText(t, item, cold).Selected == signedIn || modByText(t, item, lightning).Selected == signedIn {
			t.Errorf("signed in %v: the summed lines should be selected only when the sum is not", signedIn)
		}
	}
}

// A unique belt: 56 Life and 36 Strength make 128 total Life (two per
// Strength). A unique is priced by its own lines, so totals start unselected.
func TestPseudoTotalsOfAUniqueStartUnselected(t *testing.T) {
	raw := `Item Class: Belts
Rarity: Unique
Example Belt
Heavy Belt
--------
Requires: Level 50
--------
Item Level: 80
--------
{ Implicit Modifier — Defences }
22(20-30)% increased Stun Threshold (implicit)
--------
{ Unique Modifier — Attribute }
+36(20-40) to Strength
{ Unique Modifier — Attribute }
+29(20-40) to Dexterity
{ Unique Modifier — Life }
+56(50-60) to maximum Life
{ Unique Modifier }
When you kill a Rare monster, you gain its Modifiers for 60 seconds`
	item, err := ParseItem(raw, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	got := pseudoByID(item)
	want := map[string]float64{
		"pseudo.pseudo_total_life":       128,
		"pseudo.pseudo_total_strength":   36,
		"pseudo.pseudo_total_dexterity":  29,
		"pseudo.pseudo_total_attributes": 65,
	}
	for id, value := range want {
		mod, ok := got[id]
		if !ok || mod.Values[0] != value || mod.Selected {
			t.Errorf("%s = %+v, want %v unselected", id, mod, value)
		}
	}
	if !modByText(t, item, "+56(50-60) to maximum Life").Selected {
		t.Error("a unique's own line was unselected")
	}
}

// A hybrid mod feeds the life total through one of its lines but keeps its
// place in the search, since its Energy Shield line is in no total.
func TestPseudoTotalsCountHybridLines(t *testing.T) {
	raw := `Item Class: Helmets
Rarity: Rare
Rapture Salvation
Kamasan Tiara
-------------
Energy Shield: 503 (augmented)
## Item Level: 81
{ Prefix Modifier "Pope's" (Tier: 1) — Life, Energy Shield }
41(39-42)% increased Energy Shield
+44(42-49) to maximum Life
{ Suffix Modifier "of the Brute" (Tier: 2) — Attribute }
-3 to Strength`
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.hybrid", Text: "#% increased Energy Shield\n# to maximum Life", Type: "explicit"},
		StatEntry{ID: "explicit.str", Text: "# to Strength", Type: "explicit"},
	)
	item, err := ParseItem(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	life, ok := pseudoByID(item)["pseudo.pseudo_total_life"]
	if !ok || life.Values[0] != 38 || life.Text != "+38 total maximum Life" {
		t.Fatalf("life total = %+v, want 44 - 2*3 = 38", life)
	}
	if strength := pseudoByID(item)["pseudo.pseudo_total_strength"]; strength.Text != "-3 total to Strength" {
		t.Fatalf("negative total written as %q", strength.Text)
	}
	if !modByText(t, item, "41(39-42)% increased Energy Shield\n+44(42-49) to maximum Life").Selected {
		t.Error("the hybrid left the search though its ES line is in no total")
	}
}

// Rarity from an explicit and a fractured roll are two trade stats; the sum
// line searches both (and every other kind) as one Weighted Sum v2 total.
func TestRaritySumsAcrossKinds(t *testing.T) {
	raw := `Item Class: Rings
Rarity: Rare
Brimstone Finger
Breach Ring
--------
Item Level: 82
--------
{ Prefix Modifier "Magpie's" (Tier: 1) — Drop }
18(16-18)% increased Rarity of Items found
{ Fractured Suffix Modifier "of Plunder" (Tier: 1) — Drop }
18(16-18)% increased Rarity of Items found
--------
Fractured Item`
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.stat_3917489142", Text: "#% increased Rarity of Items found", Type: "explicit"})
	cat.Stats = append(cat.Stats, StatGroup{ID: "fractured", Entries: []StatEntry{
		{ID: "fractured.stat_3917489142", Text: "#% increased Rarity of Items found", Type: "fractured"}}})
	item, err := ParseItemWith(raw, cat, ParseOptions{SignedIn: true})
	if err != nil {
		t.Fatal(err)
	}
	sum, ok := pseudoByID(item)["sum.stat_3917489142"]
	if !ok || sum.Values[0] != 36 || !sum.Selected || len(sum.WeightStats) != 7 {
		t.Fatalf("rarity sum = %+v, want 36 selected over 7 kinds", sum)
	}
	for _, mod := range item.Mods {
		if mod.Type != "pseudo" && mod.Selected {
			t.Errorf("%s %q stayed selected beside the sum", mod.Type, mod.Text)
		}
	}
}

// Two Rarity lines merge into one, and the totals added after the merge
// once reused a key still in use; the overlay then kept drawing the previous
// item under the new item's name.
func TestModKeysStayUniqueAfterMergingAndTotals(t *testing.T) {
	raw := `Item Class: Rings
Rarity: Rare
Woe Gyre
Breach Ring
--------
Item Level: 82
--------
{ Prefix Modifier "Hoarder's" (Tier: 1) }
18(16-19)% increased Rarity of Items found
{ Suffix Modifier "of Archaeology" (Tier: 1) }
17(15-18)% increased Rarity of Items found
{ Suffix Modifier "of the Rainbow" (Tier: 1) — Elemental, Fire, Cold, Lightning, Resistance }
+16(15-16)% to all Elemental Resistances`
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.stat_3917489142", Text: "#% increased Rarity of Items found", Type: "explicit"},
		StatEntry{ID: "explicit.allres", Text: "#% to all Elemental Resistances", Type: "explicit"})
	item, err := ParseItemWith(raw, cat, ParseOptions{SignedIn: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, mod := range item.Mods {
		if seen[mod.Key] {
			t.Fatalf("key %s used twice: %+v", mod.Key, item.Mods)
		}
		seen[mod.Key] = true
	}
	if merged := item.Mods[0]; len(merged.Affixes) != 2 || merged.Affixes[0] != "prefix" || merged.Affixes[1] != "suffix" {
		t.Fatalf("merged line sides = %v, want prefix and suffix", merged.Affixes)
	}
	if sum := pseudoByID(item)["sum.stat_3917489142"]; len(sum.Values) != 1 || sum.Values[0] != 35 {
		t.Fatalf("rarity sum = %+v, want 18 + 17 = 35", sum)
	}
}
