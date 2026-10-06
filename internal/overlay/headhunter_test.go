package overlay

import (
	"strings"
	"testing"
)

const headhunterText = `Item Class: Belts
Rarity: Unique
Headhunter
Heavy Belt
--------
Requires: Level 50
--------
Item Level: 80
--------
{ Corruption Enhancement — Attribute }
+13(10-15) to Strength
--------
{ Implicit Modifier }
22(20-30)% increased Stun Threshold
{ Implicit Modifier — Charm }
Has 3(1-3) Charm Slots
--------
{ Unique Modifier — Attribute }
+36(20-40) to Strength
{ Unique Modifier — Attribute }
+29(20-40) to Dexterity
{ Unique Modifier — Life }
+56(40-60) to maximum Life
{ Unique Modifier }
When you kill a Rare monster, you gain its Modifiers for 60 seconds
--------
Corrupted`

func TestHeadhunterSearchDefaults(t *testing.T) {
	cat := testCatalog()
	cat.Stats[0].Entries = append(cat.Stats[0].Entries,
		StatEntry{ID: "explicit.stat_4080418644", Text: "+# to Strength", Type: "explicit"},
		StatEntry{ID: "explicit.stat_3261801346", Text: "+# to Dexterity", Type: "explicit"},
		StatEntry{ID: "explicit.stat_2913235441", Text: "When you kill a Rare monster, you gain its Modifiers for 60 seconds", Type: "explicit"},
	)
	cat.Stats = append(cat.Stats, StatGroup{ID: "enchant", Entries: []StatEntry{
		{ID: "enchant.strength", Text: "+# to Strength", Type: "enchant"},
	}})
	for _, corrupted := range []bool{true, false} {
		raw := headhunterText
		wantAttributes := 78.0
		if !corrupted {
			raw = strings.ReplaceAll(raw, "{ Corruption Enhancement — Attribute }\n+13(10-15) to Strength\n--------\n", "")
			raw = strings.TrimSuffix(raw, "\nCorrupted")
			wantAttributes = 65
		}
		item, err := ParseItem(raw, cat)
		if err != nil {
			t.Fatal(err)
		}
		totals := pseudoByID(item)
		attributes := totals["pseudo.pseudo_total_attributes"]
		if len(attributes.Values) != 1 || attributes.Values[0] != wantAttributes || !attributes.Selected || attributes.Hidden {
			t.Fatalf("corrupted=%v: attribute search = %+v, want %v selected and visible", corrupted, attributes, wantAttributes)
		}
		for _, mod := range item.Mods {
			if strings.Contains(mod.Text, "to Strength") || strings.Contains(mod.Text, "to Dexterity") {
				if mod.Selected {
					t.Errorf("corrupted=%v: separate attribute selected: %+v", corrupted, mod)
				}
			}
			if mod.StatID == "explicit.stat_2913235441" && (len(mod.Values) != 0 || !mod.Selected) {
				t.Errorf("presence stat must be selected without numeric bounds: %+v", mod)
			}
		}
		fixed := modByText(t, item, "When you kill a Rare monster, you gain its Modifiers for 60 seconds")
		if fixed.StatID != "explicit.stat_2913235441" || len(fixed.Values) != 0 || !fixed.Selected {
			t.Errorf("corrupted=%v: fixed stat = %+v", corrupted, fixed)
		}
		if totals["pseudo.pseudo_total_life"].Selected {
			t.Error("Headhunter's total life must remain optional")
		}
		life := modByText(t, item, "+56(40-60) to maximum Life")
		if !life.Selected || len(life.Values) != 1 || life.Values[0] != 56 {
			t.Errorf("Life roll changed: %+v", life)
		}
	}
}

func TestLiteralStatNumbersAreNotSearchValues(t *testing.T) {
	for _, text := range []string{"Grants Level 20 Example", "Gain 10 Charges on Kill"} {
		mod := ItemMod{Text: text, Type: "explicit", Values: valuesOf(text)}
		cat := Catalog{Stats: []StatGroup{{Entries: []StatEntry{{ID: "explicit.fixed", Text: text, Type: "explicit"}}}}}
		matchMod(&mod, cat, false)
		if mod.StatID != "explicit.fixed" || len(mod.Values) != 0 {
			t.Errorf("literal description acquired numeric bounds: %+v", mod)
		}
	}
}
