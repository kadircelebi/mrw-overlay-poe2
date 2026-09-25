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
