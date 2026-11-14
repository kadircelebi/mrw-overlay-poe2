package app

import (
	"testing"

	"poe2filter/internal/filter"
	"poe2filter/internal/prices"
)

// Hidden labels must not be handed to a page that loads later: it would draw
// them into a hidden window, which shows its last frame when it comes back.
func TestExpeditionViewIsEmptyOnceHidden(t *testing.T) {
	s := &AppService{}
	s.expedition.view = ExpeditionView{Seq: 4, Rows: []ExpeditionPrice{{Name: "Chaos Orb", ValueEx: 61}}}
	s.expedition.up = true
	if got := s.GetExpeditionView(); got.Seq != 4 || len(got.Rows) != 1 {
		t.Fatalf("while up: got seq %d with %d rows, want seq 4 with 1 row", got.Seq, len(got.Rows))
	}
	s.hideExpedition()
	if got := s.GetExpeditionView(); got.Seq != 4 || len(got.Rows) != 0 {
		t.Fatalf("once hidden: got seq %d with %d rows, want seq 4 with none", got.Seq, len(got.Rows))
	}
	if s.expeditionShowing() {
		t.Fatal("labels still count as showing after hiding")
	}
}

func TestValueStyleTakesTheHighestTierReached(t *testing.T) {
	cfg := filter.Config{
		ItemGroups: []filter.ItemGroup{
			{ID: "g3", Mode: filter.ItemGroupModeValue, ThresholdValue: 1, ThresholdUnit: "divine"},
			{ID: "g5", Mode: filter.ItemGroupModeValue, ThresholdValue: 10, ThresholdUnit: "divine"},
			{ID: "g4", Mode: filter.ItemGroupModeValue, ThresholdValue: 5, ThresholdUnit: "divine"},
			{ID: "list", Mode: filter.ItemGroupModeShow},
		},
		Styles: map[string]string{"user:g3": filter.CustomThemeID, "user:g4": filter.CustomThemeID, "user:g5": filter.CustomThemeID},
		CustomStyles: map[string]filter.CustomStyle{
			"user:g3": {Bg: "#101010", Text: "#ffffff", Border: "#ff0000"},
			"user:g4": {Bg: "#202020", Text: "#ffffff", Border: "#00ff00"},
			"user:g5": {Bg: "#303030", Text: "#ffffff", Border: "#0000ff"},
		},
	}
	rates := prices.Rates{DivineEx: 700}
	border := func(name string, valueEx float64) string {
		th, ok := valueStyle(cfg, nil, rates, name, valueEx)
		if !ok {
			return "none"
		}
		return cssColor(th.Border)
	}
	// 6.1 div reaches 1 and 5 Divine: the 5 Divine group, though listed
	// after the 10 Divine one.
	if got := border("Uncut Skill Gem (Level 20)", 6.1*700); got != "rgba(0,255,0,1.00)" {
		t.Fatalf("6.1 div: %s", got)
	}
	if got := border("Chaos Orb", 68); got != "none" {
		t.Fatalf("68 ex: %s", got)
	}
	if got := border("Mystic Alloy", 7000); got != "rgba(0,0,255,1.00)" {
		t.Fatalf("10 div: %s", got)
	}
	// Divine Orb keeps its own group's look whatever its worth.
	if _, ok := valueStyle(cfg, nil, rates, "Divine Orb", 700); !ok {
		t.Fatal("Divine Orb unstyled")
	}
	if got := cssColor("255 215 0 128"); got != "rgba(255,215,0,0.50)" {
		t.Fatalf("cssColor: %s", got)
	}
}
