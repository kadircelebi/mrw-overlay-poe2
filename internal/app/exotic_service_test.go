package app

import (
	"testing"

	"poe2filter/internal/filter"
)

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

func TestModifierOption(t *testing.T) {
	options := []ExoticModOption{
		{Stat: "a", Text: "+# to maximum Life", Tiers: []ExoticTier{{Tier: 1, Name: "Fecund", Min: 150}, {Tier: 2, Name: "Vigorous", Min: 120}}},
		{Stat: "b", Text: "#% increased Cast Speed", Tiers: []ExoticTier{{Tier: 1, Name: "of Sortilege", Min: 29}, {Tier: 2, Name: "Vigorous", Min: 25}}},
	}
	// A shared name goes to the option whose text reads like the line.
	o, tier, ok := modifierOption(options, "Vigorous", "26% increased Cast Speed")
	if !ok || o.Stat != "b" || tier != 2 {
		t.Errorf("shared name: got %q T%d %v", o.Stat, tier, ok)
	}
	o, tier, ok = modifierOption(options, "vigorous", "+130 to maximum Life")
	if !ok || o.Stat != "a" || tier != 2 {
		t.Errorf("life: got %q T%d %v", o.Stat, tier, ok)
	}
	if _, _, ok := modifierOption(options, "Unknown", "x"); ok {
		t.Error("unknown name matched")
	}
	if got := exoticModLabel("#% increased Cast Speed", ExoticTier{Min: 25}); got != "25+% increased Cast Speed" {
		t.Errorf("label %q", got)
	}
	if !statTextMatches("+# to maximum Life + #% increased Armour", "+130 to maximum Life") {
		t.Error("hybrid first stat not matched")
	}
}

func TestReplacedExotic(t *testing.T) {
	mod := func(stat string, names ...string) filter.ExoticEntry {
		return filter.ExoticEntry{Kind: filter.ExoticMod, Classes: []string{"Rings"}, Stat: stat, Names: names}
	}
	added := []filter.ExoticEntry{mod("rarity", "Hoarder's"), mod("rarity", "of Archaeology"), mod("life", "Fecund")}
	// Widening the prefix replaces it and leaves the suffix of the same stat.
	got := replacedExotic(added, mod("rarity", "Hoarder's", "Collector's"))
	if len(got) != 2 || got[0].Names[0] != "of Archaeology" || got[1].Names[0] != "Fecund" {
		t.Errorf("got %+v", got)
	}
	if got := replacedExotic(added, filter.ExoticEntry{Kind: filter.ExoticMod, Classes: []string{"Rings"}, Names: []string{"Hoarder's"}}); len(got) != 3 {
		t.Errorf("entry without stat replaced something: %+v", got)
	}
}
