package collector

import (
	"testing"

	"poe2filter/internal/prices"
)

// poe2scout is the source the thresholds were tuned against, so the merge may
// only fill holes. Overwriting a price here would quietly move every rule that
// depends on it.
func TestMergeExchangeOnlyFillsHoles(t *testing.T) {
	have := []prices.CurrencyPrice{
		{Name: "Chaos Orb", Category: "currency", ValueEx: 10},
		{Name: "Arakaali's Lust", Category: "lineagesupportgems", ValueEx: 4},
	}
	add := []NinjaExchangePrice{
		{Name: "Chaos Orb", Category: "currency", ValueDiv: 1}, // already known
		{Name: "arakaali's lust", Category: "x", ValueDiv: 1},  // same name, other case
		{Name: "Helbrym's Hide", Category: "lineagesupportgems", ValueDiv: 0.005},
	}
	out := MergeExchangePrices(have, add, 400)

	if len(out) != 3 {
		t.Fatalf("expected one addition, got %d entries: %+v", len(out), out)
	}
	if out[0].ValueEx != 10 || out[1].ValueEx != 4 {
		t.Error("an existing price must not be touched")
	}
	got := out[2]
	if got.Name != "Helbrym's Hide" || got.Category != "lineagesupportgems" {
		t.Errorf("wrong entry added: %+v", got)
	}
	if want := 0.005 * 400; got.ValueEx != want {
		t.Errorf("divine should be converted to exalted: got %v, want %v", got.ValueEx, want)
	}
}

// Without a rate the divine values cannot be converted, and guessing would be
// worse than leaving the holes open.
func TestMergeExchangeNeedsARate(t *testing.T) {
	have := []prices.CurrencyPrice{{Name: "Chaos Orb", ValueEx: 10}}
	out := MergeExchangePrices(have, []NinjaExchangePrice{{Name: "Helbrym's Hide", ValueDiv: 1}}, 0)
	if len(out) != 1 {
		t.Errorf("nothing should be added without a divine rate: %+v", out)
	}
}
