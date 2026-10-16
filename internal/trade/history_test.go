package trade

import (
	"encoding/json"
	"testing"
)

func TestPlainModText(t *testing.T) {
	cases := map[string]string{
		"34% increased [Spell] Damage":                "34% increased Spell Damage",
		"[AoESkill|AoE] of [Cold] skills":             "AoE of Cold skills",
		"+12 to maximum Life":                         "+12 to maximum Life",
		"[Repeat|Repeatable], [Spell]":                "Repeatable, Spell",
		"Gain 10% of [Damage|damage] as extra [Fire]": "Gain 10% of damage as extra Fire",
	}
	for in, want := range cases {
		if got := PlainModText(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// Mod lines come as objects or as plain text, even within one item.
func TestTradeHistoryAnswerReadsBothModForms(t *testing.T) {
	raw := `{"result":[{"item_id":"x1","time":"2026-10-14T08:41:14+00:00","price":{"amount":2,"currency":"divine"},
		"item":{"name":"Glyph Whorl","typeLine":"Mnemonic Ring","rarity":"Rare","ilvl":82,
		"enchantMods":["Allocates [Spell|Spellcraft]"],
		"explicitMods":[{"description":"34% increased [Spell] Damage"},"+12 to maximum Life"]}}]}`
	var answer historyAnswer
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatal(err)
	}
	it := answer.Result[0].Item
	got := []string{PlainModText(it.EnchantMods[0].Description), PlainModText(it.ExplicitMods[0].Description), it.ExplicitMods[1].Description}
	want := []string{"Allocates Spellcraft", "34% increased Spell Damage", "+12 to maximum Life"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mod %d: %q, want %q", i, got[i], want[i])
		}
	}
}
