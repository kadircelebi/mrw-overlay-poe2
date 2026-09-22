package collector

import "testing"

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
