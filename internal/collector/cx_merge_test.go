package collector

import (
	"testing"

	"poe2filter/internal/prices"
)

// The exchange's price replaces another source's, a weak one does not, and
// an item only the exchange lists is added with a category from its class.
func TestMergeCXPrices(t *testing.T) {
	have := []prices.CurrencyPrice{
		{Name: "Orb of Annulment", APIID: "annul", Category: "currency", ValueEx: 370},
		{Name: "Stag Idol", Category: "idol", ValueEx: 0.5},
		{Name: "Regal Shard", Category: "currency", ValueEx: 0.1},
	}
	cx := CXResult{
		Items: map[string]prices.CurrencyPrice{
			"Orb of Annulment":            {Name: "Orb of Annulment", ValueEx: 326, Markets: []prices.MarketPrice{{Base: "divine", ValueEx: 326, Units: 11565}}},
			"Stag Idol":                   {Name: "Stag Idol", ValueEx: 25.6, Markets: []prices.MarketPrice{{Base: "chaos", ValueEx: 25.6, Units: 3}}},
			"Uncut Support Gem (Level 5)": {Name: "Uncut Support Gem (Level 5)", ValueEx: 12},
		},
		Classes: map[string]string{"Uncut Support Gem (Level 5)": "UncutSupportGemStackable"},
		Weak:    map[string]bool{"Stag Idol": true},
	}
	out, notes := MergeCXPrices(have, cx)
	by := map[string]prices.CurrencyPrice{}
	for _, c := range out {
		by[c.Name] = c
	}
	if a := by["Orb of Annulment"]; a.ValueEx != 326 || a.APIID != "annul" || a.Category != "currency" {
		t.Errorf("annulment %+v", a)
	}
	if s := by["Stag Idol"]; s.ValueEx != 0.5 || len(s.Markets) != 1 {
		t.Errorf("weak item %+v", s)
	}
	if r := by["Regal Shard"]; r.ValueEx != 0.1 {
		t.Errorf("untraded item %+v", r)
	}
	if g := by["Uncut Support Gem (Level 5)"]; g.ValueEx != 12 || g.Category != "uncutgems" {
		t.Errorf("added item %+v", g)
	}
	if len(notes) != 0 {
		t.Errorf("annulment is within 50%%, nothing to note: %v", notes)
	}
}
