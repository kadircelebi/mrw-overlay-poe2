package app

import (
	"testing"
	"time"

	"poe2filter/internal/invscan"
	"poe2filter/internal/lootlog"
	"poe2filter/internal/prices"
)

func TestFarmFillFindsTheBestMapAndTheTopItems(t *testing.T) {
	start := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	run := func(items ...lootlog.Item) []lootlog.Run { return []lootlog.Run{{Items: items}} }
	list := []lootlog.Map{ // newest first
		{At: start.Add(2 * time.Hour), AreaID: "MapCenotes", ValueEx: 900, Runs: run(
			lootlog.Item{Name: "Divine Orb", Count: 1, EachEx: 700},
			lootlog.Item{Name: "Exalted Orb", Count: 200, EachEx: 1},
		)},
		{At: start.Add(time.Hour), AreaID: "MapWaywardIsle", ValueEx: 1500, Runs: run(
			lootlog.Item{Name: "Divine Orb", Count: 2, EachEx: 690},
			lootlog.Item{Name: "Exalted Orb", Count: 120, EachEx: 1},
		)},
	}
	var v FarmView
	farmFill(&v, list)
	if v.Best == nil || v.Best.Name != "Wayward Isle" || v.Best.ValueEx != 1500 {
		t.Errorf("best %+v", v.Best)
	}
	if len(v.Top) != 2 || v.Top[0].Name != "Divine Orb" || v.Top[0].Count != 3 || v.Top[0].EachEx != 700 || v.Top[1].Count != 320 {
		t.Errorf("top %+v", v.Top)
	}
}

func TestOnlyPricedTakesTheOnePricedOfASharedIcon(t *testing.T) {
	snap := &prices.Snapshot{Currency: []prices.CurrencyPrice{
		{Name: "Exceptional Verisium", ValueEx: 4.6},
		{Name: "Uncut Skill Gem (Level 19)", ValueEx: 17},
		{Name: "Uncut Skill Gem (Level 20)", ValueEx: 90},
	}}
	verisium := []invscan.Entry{{Name: "Exceptional Verisium"}, {Name: "Mutated Verisium"}, {Name: "Corrupted Verisium"}, {Name: "Founders Verisium"}}
	if e, ok := onlyPriced(verisium, snap); !ok || e.Name != "Exceptional Verisium" {
		t.Errorf("verisium: %v %v", e, ok)
	}
	gems := []invscan.Entry{{Name: "Uncut Skill Gem (Level 19)"}, {Name: "Uncut Skill Gem (Level 20)"}}
	if _, ok := onlyPriced(gems, snap); ok {
		t.Error("two priced gem levels taken for one")
	}
}

func TestAFrameIsReadOnceTheGridStandsStill(t *testing.T) {
	var st autoScanState
	at := func(x float64) invscan.Grid { return invscan.Grid{X: x, Y: 26, Cell: 105, Cols: 12, Rows: 5} }
	// The panel slides in: 40, 30, 24.6, then stands at 24.3.
	for i, tc := range []struct {
		x    float64
		want bool
	}{{40, false}, {30, false}, {24.6, false}, {24.3, true}, {24.3, true}} {
		if got := st.settled(at(tc.x)); got != tc.want {
			t.Errorf("frame %d at x %.1f: settled %v, want %v", i, tc.x, got, tc.want)
		}
	}
}

func TestAnEmptyInventoryIsABase(t *testing.T) {
	var st autoScanState
	empty := map[string]int{}
	if st.newBase(empty, 0, false, false) {
		t.Error("one read taken for a base")
	}
	if !st.newBase(map[string]int{}, 0, false, false) {
		t.Error("two empty reads in a row not taken for a base")
	}
	if st.newBase(map[string]int{}, 0, false, true) {
		t.Error("the same base kept again")
	}
	if !st.newBase(map[string]int{}, 0, false, false) {
		t.Error("the same read not kept again once the loot log was cleared")
	}
	st.newBase(map[string]int{"Exalted Orb": 3}, 0, false, true)
	if !st.newBase(map[string]int{"Exalted Orb": 3}, 0, false, true) {
		t.Error("a changed inventory not kept")
	}
}
