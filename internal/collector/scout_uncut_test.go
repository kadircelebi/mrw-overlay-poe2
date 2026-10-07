package collector

import (
	"encoding/json"
	"testing"
)

func TestUncutGemPrices(t *testing.T) {
	// Shaped like poe2scout's item list of 2026-10-07.
	raw := `[
	 {"CategoryApiId":"uncutgems","Text":"Uncut Spirit Gem (Level 20)","ApiId":"uncut-spirit-gem-20","CurrentPrice":5383.68},
	 {"CategoryApiId":"uncutgems","Text":"Uncut Skill Gem (Level 19)","ApiId":"uncut-skill-gem-19","CurrentPrice":24.7},
	 {"CategoryApiId":"uncutgems","Text":"Uncut Support Gem (Level 1)","ApiId":"uncut-support-gem-1","CurrentPrice":0},
	 {"CategoryApiId":"currency","Text":"Chaos Orb","ApiId":"chaos","CurrentPrice":68}
	]`
	var items []scoutItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatal(err)
	}
	got := uncutGemPrices(items)
	if len(got) != 2 || got["Uncut Spirit Gem (Level 20)"] != 5383.68 || got["Uncut Skill Gem (Level 19)"] != 24.7 {
		t.Fatalf("got %v", got)
	}
}

func TestScoutCurrencyTakesUncutSkillAndSpiritGems(t *testing.T) {
	raw := `[
	 {"CategoryApiId":"uncutgems","Text":"Uncut Spirit Gem (Level 20)","ApiId":"uncut-spirit-gem-20","CurrentPrice":5383.68},
	 {"CategoryApiId":"uncutgems","Text":"Uncut Skill Gem (Level 1)","ApiId":"uncut-skill-gem-1","CurrentPrice":944.8},
	 {"CategoryApiId":"uncutgems","Text":"Uncut Support Gem (Level 4)","ApiId":"uncut-support-gem-4","CurrentPrice":77.5},
	 {"CategoryApiId":"waystones","Text":"Waystone (Tier 15)","ApiId":"waystone-15","CurrentPrice":3},
	 {"CategoryApiId":"currency","Text":"Divine Orb","ApiId":"divine","CurrentPrice":699}
	]`
	var items []scoutItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatal(err)
	}
	got, divineEx := scoutCurrency(items)
	names := map[string]string{}
	for _, c := range got {
		names[c.Name] = c.Category
	}
	if divineEx != 699 || names["Uncut Spirit Gem (Level 20)"] != "uncutgems" || names["Uncut Skill Gem (Level 1)"] != "uncutgems" {
		t.Fatalf("got %v, divine %v", names, divineEx)
	}
	if _, ok := names["Uncut Support Gem (Level 4)"]; ok {
		t.Fatal("support gems stay with their level slider")
	}
	if _, ok := names["Waystone (Tier 15)"]; ok {
		t.Fatal("waystones are skipped")
	}
}
