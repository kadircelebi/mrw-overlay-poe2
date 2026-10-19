package app

import (
	"testing"
	"time"

	"poe2filter/internal/lootlog"
)

func TestFarmFillTakesTheSessionsMapsOnly(t *testing.T) {
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
		{At: start.Add(-time.Hour), AreaID: "MapCrypt", ValueEx: 9000, Runs: run(
			lootlog.Item{Name: "Mirror of Kalandra", Count: 1, EachEx: 9000},
		)},
	}
	var v FarmView
	farmFill(&v, list, start)
	if v.Best == nil || v.Best.Name != "Wayward Isle" || v.Best.ValueEx != 1500 {
		t.Errorf("best %+v", v.Best)
	}
	if len(v.Top) != 2 || v.Top[0].Name != "Divine Orb" || v.Top[0].Count != 3 || v.Top[0].EachEx != 700 || v.Top[1].Count != 320 {
		t.Errorf("top %+v", v.Top)
	}
}
