package overlay

import (
	"testing"

	"poe2filter/internal/prices"
)

func TestQuoteCurrency(t *testing.T) {
	snap := &prices.Snapshot{
		Rates:    prices.Rates{DivineEx: 400, ChaosEx: 8},
		Currency: []prices.CurrencyPrice{{Name: "Orb of Annulment", Category: "Currency", ValueEx: 30}},
	}
	if q := QuoteCurrency(snap, "orb of annulment"); !q.Found || q.ValueEx != 30 || q.Name != "Orb of Annulment" || q.DivineEx != 400 {
		t.Errorf("listed currency: %+v", q)
	}
	for name, want := range map[string]float64{"Exalted Orb": 1, "Divine Orb": 400, "Chaos Orb": 8} {
		if q := QuoteCurrency(snap, name); !q.Found || q.ValueEx != want {
			t.Errorf("%s from rates: %+v", name, q)
		}
	}
	if q := QuoteCurrency(snap, "Slipstrike Vest"); q.Found {
		t.Errorf("gear must not be quoted: %+v", q)
	}
	if q := QuoteCurrency(nil, "Chaos Orb"); q.Found {
		t.Errorf("no snapshot, no quote: %+v", q)
	}
}

func TestStackSize(t *testing.T) {
	for in, want := range map[string]int{" 808/5000": 808, " 1,234/5,000": 1234, " 3/10": 3, "": 0} {
		if got := stackSize(in); got != want {
			t.Errorf("stackSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestQuoteCurrencyConvertsWithTheListsOwnDivine(t *testing.T) {
	snap := &prices.Snapshot{
		Rates:    prices.Rates{DivineEx: 490, ChaosEx: 63},
		Currency: []prices.CurrencyPrice{{Name: "Divine Orb", ValueEx: 570}, {Name: "Chaos Orb", ValueEx: 74}, {Name: "Orb of Annulment", ValueEx: 400}},
	}
	if q := QuoteCurrency(snap, "Orb of Annulment"); q.DivineEx != 570 || q.ChaosEx != 74 {
		t.Errorf("mixed sources: %+v", q)
	}
}

func TestExceptionalPrefixIsNotPartOfTheBase(t *testing.T) {
	catalog := Catalog{Items: []ItemGroup{{Entries: []ItemEntry{{Type: "Hawker's Jacket"}, {Type: "Exceptional Verisium"}, {Type: "Verisium"}}}}}
	raw := "Item Class: Body Armours\nRarity: Normal\nExceptional Hawker's Jacket\n--------\nEvasion Rating: 237\nEnergy Shield: 73\n--------\nRequires: Level 62, 52 (augmented) Dex, 52 (augmented) Int\n--------\nSockets: S S S \n--------\nItem Level: 82\n"
	item, err := ParseItem(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if item.BaseType != "Hawker's Jacket" || item.RuneSockets != 3 || !item.Exceptional {
		t.Errorf("base %q sockets %d", item.BaseType, item.RuneSockets)
	}
	currency, err := ParseItem("Item Class: Stackable Currency\nRarity: Currency\nExceptional Verisium\n--------\nStack Size: 4/20\n", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if currency.BaseType != "Exceptional Verisium" || currency.StackSize != 4 {
		t.Errorf("real currency name changed: %q stack %d", currency.BaseType, currency.StackSize)
	}
}

// Items that trade only on the currency exchange are marked; waystones, which
// the item search prices by tier, and anything with a rarity of its own are not.
func TestExchangeItemsAreMarked(t *testing.T) {
	catalog := Catalog{Currencies: []CurrencyEntry{
		{ID: "an-audience-with-the-king", Text: "An Audience with the King", Group: "Ritual"},
		{ID: "waystone-15", Text: "Waystone (Tier 15)", Group: "Waystones"},
	}}
	for raw, want := range map[string]string{
		"Item Class: Map Fragments\nRarity: Normal\nAn Audience with the King\n--------\nCan be used in a personal Map Device.\n": "an-audience-with-the-king",
		"Item Class: Waystones\nRarity: Normal\nWaystone (Tier 15)\n--------\nWaystone Tier: 15\n":                                "",
		"Item Class: Waystones\nRarity: Rare\nDark Road\nWaystone (Tier 15)\n--------\nWaystone Tier: 15\n":                       "",
	} {
		item, err := ParseItem(raw, catalog)
		if err != nil {
			t.Fatal(err)
		}
		if item.Exchange != want {
			t.Errorf("%q: exchange %q, want %q", item.BaseType, item.Exchange, want)
		}
	}
}

// Uncut skill and spirit gems trade on the currency exchange and are priced
// by level: they get the worth card and their level (the filter's GemLevel).
// Support gems, which the price list leaves out, keep the item search.
func TestUncutGemsAreExchangeItemsWithTheirLevel(t *testing.T) {
	catalog := Catalog{Currencies: []CurrencyEntry{
		{ID: "uncut-spirit-gem-20", Text: "Uncut Spirit Gem (Level 20)", Group: "UncutGems"},
		{ID: "uncut-support-gem-4", Text: "Uncut Support Gem (Level 4)", Group: "UncutGems"},
	}}
	for raw, want := range map[string]struct {
		exchange string
		level    int
	}{
		"Item Class: Uncut Spirit Gems\nRarity: Currency\nUncut Spirit Gem (Level 20)\n--------\nStack Size: 1/1\n":  {"uncut-spirit-gem-20", 20},
		"Item Class: Uncut Support Gems\nRarity: Currency\nUncut Support Gem (Level 4)\n--------\nStack Size: 1/1\n": {"", 4},
	} {
		item, err := ParseItem(raw, catalog)
		if err != nil {
			t.Fatal(err)
		}
		if item.Exchange != want.exchange || item.GemLevel != want.level {
			t.Errorf("%q: exchange %q level %d, want %+v", item.BaseType, item.Exchange, item.GemLevel, want)
		}
	}
}
