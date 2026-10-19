package app

import (
	"context"
	"log"
	"slices"
	"strings"
	"time"

	"poe2filter/internal/lootlog"
	"poe2filter/internal/overlay"
)

// The farm's figures for the loot session: its times from the game's log
// (lootlog.FarmTimes), the best map and what came in most from the loot
// log. The rates (per map, per hour) are made from these where they show.

// FarmView is the loot session's figures. Times are in seconds, as they
// stood at At (Unix ms); while Live is a map or a counted stay at home,
// the clocks run on from there (a stay at most RestLeft more). Since (Unix
// ms) is when that map trip or stay began. Timed tells the game's log was
// read. LastMap is the latest map left, LastSecs its time. Top is what the
// session's maps brought, most worth first.
type FarmView struct {
	At       int64          `json:"at"`
	Timed    bool           `json:"timed"`
	Live     string         `json:"live"`
	Since    int64          `json:"since"`
	RestLeft float64        `json:"restLeft"`
	Active   float64        `json:"active"`
	InMaps   float64        `json:"inMaps"`
	InRest   float64        `json:"inRest"`
	Breaks   float64        `json:"breaks"`
	Maps     int            `json:"maps"`
	LastMap  string         `json:"lastMap"`
	LastSecs float64        `json:"lastSecs"`
	Best     *FarmBest      `json:"best"`
	Top      []lootlog.Item `json:"top"`
}

// FarmBest is the session's map that brought the most.
type FarmBest struct {
	Name    string  `json:"name"`
	ValueEx float64 `json:"valueEx"`
}

// farmTop is how many kinds of item the session's list shows.
const farmTop = 12

// farmTimes reads the game's log and keeps the time of each map left in
// the loot log; the view's figures from the loot log come with farmFill.
func (s *AppService) farmTimes(league string, start time.Time) FarmView {
	now := time.Now()
	v := FarmView{At: now.UnixMilli()}
	path, _ := s.gameLog.Load().(string)
	if path == "" {
		return v
	}
	marks, _, last := s.farmLog.Marks(path)
	if len(marks) == 0 {
		return v
	}
	running := true
	if _, ok := overlay.GameClient(); !ok && !last.IsZero() {
		// The game is closed: the clock stopped at its last line.
		now, running = last, false
	}
	in := make([]lootlog.Mark, len(marks))
	for i, m := range marks {
		in[i] = lootlog.Mark{At: m.At, AFK: m.AFK}
		if m.Area != nil {
			a := lootArea(*m.Area)
			in[i].Area = &a
		}
	}
	t := lootlog.FarmTimes(in, start, now)
	v.Timed = true
	v.Active, v.InMaps, v.InRest, v.Breaks = t.Active.Seconds(), t.InMaps.Seconds(), t.InRest.Seconds(), t.Breaks.Seconds()
	v.Maps = len(t.Maps)
	if running {
		v.Live, v.RestLeft = t.Live, t.RestLeft.Seconds()
		v.Since = t.Since.UnixMilli()
	}
	left := t.Maps
	if t.Live == lootlog.LiveMap && len(left) > 0 {
		left = left[:len(left)-1]
	}
	if n := len(left); n > 0 {
		v.LastMap, v.LastSecs = mapName(left[n-1].AreaID), left[n-1].Dur.Seconds()
	}
	if _, err := s.lootLog.SetMapTimes(league, left); err != nil {
		log.Printf("loot log: map times: %v", err)
	}
	return v
}

// farmFill adds the best map and what came in most, from the session's
// maps (newest first, as the loot log lists them).
func farmFill(v *FarmView, list []lootlog.Map, start time.Time) {
	type kind struct {
		count  int
		eachEx float64
	}
	kinds := map[string]*kind{}
	for _, m := range list {
		if m.At.Before(start) {
			continue
		}
		if m.ValueEx > 0 && (v.Best == nil || m.ValueEx > v.Best.ValueEx) {
			v.Best = &FarmBest{Name: mapName(m.AreaID), ValueEx: m.ValueEx}
		}
		for _, r := range m.Runs {
			for _, it := range r.Items {
				k := kinds[it.Name]
				if k == nil {
					// The newest worth: maps come newest first.
					k = &kind{eachEx: it.EachEx}
					kinds[it.Name] = k
				}
				k.count += it.Count
			}
		}
	}
	for name, k := range kinds {
		v.Top = append(v.Top, lootlog.Item{Name: name, Count: k.count, EachEx: k.eachEx})
	}
	slices.SortFunc(v.Top, func(a, b lootlog.Item) int {
		wa, wb := a.EachEx*float64(a.Count), b.EachEx*float64(b.Count)
		switch {
		case wa != wb:
			if wa > wb {
				return -1
			}
			return 1
		case a.Count != b.Count:
			return b.Count - a.Count
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(v.Top) > farmTop {
		v.Top = v.Top[:farmTop]
	}
}

// farmLoop sends the figures again while the loot box or the tracker
// shows them, for the times to follow the game's log.
func (s *AppService) farmLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if s.app == nil {
			continue
		}
		if w := s.lootBoxWindow; w != nil && w.IsVisible() {
			s.app.Event.Emit("loot-status", s.GetLootStatus())
		}
		if w := s.trackerWindow; w != nil && w.IsVisible() {
			s.app.Event.Emit("loot-log", s.GetLootLog())
		}
	}
}
