package filtereval

import (
	"testing"

	"poe2filter/internal/overlay"
)

func TestFactsFromItem(t *testing.T) {
	raw := "Item Class: Rings\nRarity: Rare\nEntropy Eye\nPrismatic Ring\n--------\nItem Level: 81\n--------\n" +
		"{ Implicit Modifier — Elemental, Resistance }\n+9(7-10)% to all Elemental Resistances\n--------\n" +
		"{ Prefix Modifier \"Spirited\" — Mana, Caster }\n29(26-32)% increased effect of Arcane Surge on you\n" +
		"{ Prefix Modifier \"Crackling\" (Tier: 6) — Damage }\nAdds 1 to 23(23-27) Lightning damage to Attacks\n" +
		"{ Desecrated Suffix Modifier \"of Sortilege\" (Tier: 2) — Caster, Speed }\n21(19-21)% increased Cast Speed\n"
	item, err := overlay.ParseItem(raw, overlay.Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	f := FactsFromItem(item)
	if f.Class != "Rings" || f.BaseType != "Prismatic Ring" || f.Rarity != "Rare" || f.ItemLevel != 81 || !f.Identified {
		t.Errorf("facts = %+v", f)
	}
	if len(f.ExplicitMods) != 3 || f.ExplicitMods[0] != "Spirited" || f.ExplicitMods[2] != "of Sortilege" {
		t.Errorf("mods = %v", f.ExplicitMods)
	}

	cur, _ := overlay.ParseItem("Item Class: Stackable Currency\nRarity: Currency\nDivine Orb\n--------\nStack Size: 3/20\n", overlay.Catalog{})
	if f := FactsFromItem(cur); f.Rarity != "Normal" || f.StackSize != 3 || f.BaseType != "Divine Orb" {
		t.Errorf("currency facts = %+v", f)
	}
	way, _ := overlay.ParseItem("Item Class: Waystones\nRarity: Normal\nWaystone (Tier 15)\n--------\nWaystone Tier: 15\n", overlay.Catalog{})
	if f := FactsFromItem(way); f.WaystoneTier != 15 {
		t.Errorf("waystone facts = %+v", f)
	}
}
