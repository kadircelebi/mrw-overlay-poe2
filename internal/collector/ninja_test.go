package collector

import (
	"encoding/json"
	"testing"
)

func TestGroundBaseExcludesCraftedVariants(t *testing.T) {
	tests := map[string]string{
		"Tense Crossbow":                "Tense Crossbow",
		`"Tense Crossbow"`:              "Tense Crossbow",
		"Runeforged Tense Crossbow":     "",
		"Runemastered Tense Crossbow":   "",
		`"Runemastered Tense Crossbow"`: "",
	}
	for input, want := range tests {
		if got := GroundBase(input); got != want {
			t.Errorf("GroundBase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNinjaIconReachesTheSnapshot(t *testing.T) {
	var data ninjaResponse
	raw := `{"core":{"rates":{}},"lines":[{"name":"Pragmatism","baseType":"Explorer Armour","primaryValue":2,"listingCount":5,"icon":"https://web.poecdn.com/x/Pragmatism.png"}]}`
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	line := data.Lines[0]
	res := NinjaResult{Uniques: map[string][]NinjaUnique{line.BaseType: {{Name: line.Name, ValueDiv: line.PrimaryValue, Listings: line.ListingCount, Icon: line.Icon}}}}
	got := res.ToExalted(100)["Explorer Armour"]
	if len(got) != 1 || got[0].Icon != "https://web.poecdn.com/x/Pragmatism.png" || got[0].ValueEx != 200 {
		t.Fatalf("uniques = %+v", got)
	}
}
