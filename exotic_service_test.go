package main

import "testing"

func TestNamedModifiers(t *testing.T) {
	raw := "Item Class: Rings\nRarity: Rare\nEntropy Eye\nPrismatic Ring\n--------\n" +
		"{ Implicit Modifier — Elemental }\n+9(7-10)% to all Elemental Resistances\n--------\n" +
		"{ Prefix Modifier \"Spirited\" — Mana, Caster }\n29(26-32)% increased effect of Arcane Surge on you\n" +
		"{ Desecrated Suffix Modifier \"of Sortilege\" (Tier: 2) — Caster, Speed }\n21(19-21)% increased Cast Speed\n"
	got := namedModifiers(raw)
	if len(got) != 2 || got[0].name != "Spirited" || got[0].text != "29% increased effect of Arcane Surge on you" || got[1].name != "of Sortilege" || got[1].text != "21% increased Cast Speed" {
		t.Errorf("got %+v", got)
	}
}
