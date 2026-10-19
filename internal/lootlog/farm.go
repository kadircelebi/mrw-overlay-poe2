package lootlog

import "time"

// The farm's times come from the game's log: when each area was entered
// and when the game marked the player away (AFK). Time in a map counts in
// full, the areas entered from it with it. Time in a hideout or town counts
// less what the game marked away, and of one stay there (from a map to the
// next, towns and hideouts alike) at most BreakAfter: past that the player
// is taken to be on a break, and the session's clock stops until they go
// on.

// BreakAfter is how much of one stay in a hideout or town counts.
const BreakAfter = 10 * time.Minute

// Mark is one line of the log the times are cut at: an area entered (Area
// set), or the game marking the player away (AFK) or back (Area nil).
type Mark struct {
	At   time.Time
	Area *Area
	AFK  bool
}

// MapTime is the time spent in one map: from each entry from a hideout or
// town to the way back, entries into the same instance in a row added up.
type MapTime struct {
	AreaID string
	Seed   string
	At     time.Time // first entered
	Dur    time.Duration
}

// Where the player is, as the clock sees it.
const (
	LiveNone  = ""
	LiveMap   = "map"
	LiveRest  = "rest"  // in a hideout or town, counted
	LiveBreak = "break" // away, or past BreakAfter
)

// Times are the farm's times between since and now. Active is InMaps plus
// InRest; Breaks is the time in hideouts and towns that does not count.
// Maps are oldest first. Live says where the player is now: Since is when
// that map trip or stay began, RestLeft how much more of the stay counts.
type Times struct {
	Active   time.Duration
	InMaps   time.Duration
	InRest   time.Duration
	Breaks   time.Duration
	Maps     []MapTime
	Live     string
	Since    time.Time
	RestLeft time.Duration
}

// FarmTimes cuts the session's marks (oldest first) into the farm's times
// from since to now.
func FarmTimes(marks []Mark, since, now time.Time) Times {
	var t Times
	// The away spans, the last one maybe still open.
	type span struct{ from, to time.Time }
	var away []span
	for _, m := range marks {
		if m.Area != nil || !m.At.Before(now) {
			continue
		}
		switch {
		case m.AFK && (len(away) == 0 || !away[len(away)-1].to.IsZero()):
			away = append(away, span{from: m.At})
		case !m.AFK && len(away) > 0 && away[len(away)-1].to.IsZero():
			away[len(away)-1].to = m.At
		}
	}
	awayIn := func(from, to time.Time) time.Duration {
		var d time.Duration
		for _, s := range away {
			end := s.to
			if end.IsZero() {
				end = now
			}
			if lo, hi := maxTime(from, s.from), minTime(to, end); hi.After(lo) {
				d += hi.Sub(lo)
			}
		}
		return d
	}
	var areas []Mark
	for _, m := range marks {
		if m.Area != nil && m.At.Before(now) {
			areas = append(areas, m)
		}
	}
	var stay time.Duration // counted of the current stay at home
	prevRest := true
	// The map the current or last trip went into, when first entered, and
	// its place in t.Maps (-1: not counted yet).
	var trip [2]string
	var tripAt time.Time
	cur := -1
	for i, m := range areas {
		end := now
		if i+1 < len(areas) {
			end = areas[i+1].At
		}
		from := maxTime(m.At, since)
		a := *m.Area
		if a.Rest {
			if !prevRest {
				stay = 0
				t.Since = m.At
			}
			prevRest = true
			if !end.After(from) {
				continue
			}
			d := end.Sub(from)
			counted := min(d-awayIn(from, end), BreakAfter-stay)
			stay += counted
			t.InRest += counted
			t.Breaks += d - counted
			continue
		}
		if prevRest {
			// A trip from home: into another map, or the one before again
			// through its portal.
			if key := [2]string{a.ID, a.Seed}; key != trip {
				trip, tripAt, cur = key, m.At, -1
			}
			t.Since = m.At
		}
		prevRest = false
		if end.After(from) {
			if cur < 0 {
				t.Maps = append(t.Maps, MapTime{AreaID: trip[0], Seed: trip[1], At: tripAt})
				cur = len(t.Maps) - 1
			}
			t.Maps[cur].Dur += end.Sub(from)
			t.InMaps += end.Sub(from)
		}
	}
	t.Active = t.InMaps + t.InRest
	switch {
	case len(areas) == 0:
	case !prevRest:
		t.Live = LiveMap
	case stay >= BreakAfter || awayIn(now.Add(-time.Second), now) > 0:
		t.Live = LiveBreak
	default:
		t.Live, t.RestLeft = LiveRest, BreakAfter-stay
	}
	if t.Since.IsZero() && len(areas) > 0 {
		t.Since = areas[0].At
	}
	return t
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
