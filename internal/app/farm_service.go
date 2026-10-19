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

// The farm's figures for a loot session: its times from the game's log
// (lootlog.Session.Clock), the best map and what came in most from the loot
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

// followLog reads what the game's log added since the last read into the
// loot log's open session, while the inventory scan is on.
func (s *AppService) followLog() {
	if mode, _ := s.invScanMode(); mode == overlay.ScanOff {
		return
	}
	path, _ := s.gameLog.Load().(string)
	if path == "" {
		return
	}
	s.farmFollow.Lock()
	defer s.farmFollow.Unlock()
	marks, next, again, err := overlay.ReadMarks(path, overlay.LogPos(s.lootLog.LogPos()))
	if err != nil {
		return
	}
	lines := make([]lootlog.Line, len(marks))
	for i, m := range marks {
		lines[i] = lootlog.Line{Mark: lootlog.Mark{At: m.At, AFK: m.AFK}, Open: m.Open, Prev: m.Prev}
		if m.Area != nil {
			a := lootArea(*m.Area)
			lines[i].Area = &a
		}
	}
	if err := s.lootLog.Follow(s.overlayClient.League(), lines, lootlog.LogPos(next), again); err != nil {
		log.Printf("loot log: follow: %v", err)
	}
}

// farmNow is the time the open session's clock stands at: now while the
// game runs, else the log's last line; running tells which.
func (s *AppService) farmNow() (time.Time, bool) {
	if _, ok := overlay.GameClient(); !ok {
		if last := s.lootLog.LogPos().Last; !last.IsZero() {
			return last, false
		}
	}
	return time.Now(), true
}

// farmTimes is the figures of session sess, with maps its maps (newest
// first) and now as farmNow gives it. The time of each map the open session
// has left is kept in the loot log.
func (s *AppService) farmTimes(league string, sess lootlog.Session, maps []lootlog.Map, now time.Time, running bool) FarmView {
	t, sum := sess.Clock(now)
	v := FarmView{At: now.UnixMilli(), Timed: sum.Active > 0 || len(sess.Marks) > 0}
	v.Active, v.InMaps, v.InRest, v.Breaks = float64(sum.Active), float64(sum.InMaps), float64(sum.InRest), float64(sum.Breaks)
	v.Maps = sum.Maps
	if sess.Open() {
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
	}
	if v.LastMap == "" {
		for _, m := range maps {
			if m.Secs > 0 {
				v.LastMap, v.LastSecs = mapName(m.AreaID), float64(m.Secs)
				break
			}
		}
	}
	farmFill(&v, maps)
	return v
}

// farmFill adds the best map and what came in most, from the session's
// maps (newest first, as the loot log lists them).
func farmFill(v *FarmView, list []lootlog.Map) {
	type kind struct {
		count  int
		eachEx float64
	}
	kinds := map[string]*kind{}
	for _, m := range list {
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
		s.followLog()
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
