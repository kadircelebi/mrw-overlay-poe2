package lootlog

import (
	"slices"
	"strings"
	"time"
)

// A session is a stretch of farming: the maps first entered in it and the
// time the game's log says was spent. One begins when the player starts it,
// or by itself when the game starts again after it was closed for at least
// NewSessionAfter; after a shorter close (a crash) the session goes on and
// the time the game was closed counts for nothing. Sessions are kept by
// league, oldest first. A map belongs to the session it was first entered
// in, from its start to the next one's. The open session keeps the log's
// marks as they come, so the app does not read the log again after a
// restart; a closed one keeps only its times.

// NewSessionAfter is how long the game must have been closed for its next
// start to begin a new session.
const NewSessionAfter = time.Hour

// sessionNameMax is the longest name a session takes, in characters.
const sessionNameMax = 40

// Session is one session. Times are those not in Marks: a closed session's
// all, an open one's from the sessions merged into it. Cost is what the
// player put into it (waystones, tablets…) as they told it.
type Session struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end,omitzero"`
	Name  string    `json:"name,omitempty"`
	Cost  *Cost     `json:"cost,omitempty"`
	Marks []Mark    `json:"marks,omitempty"`
	Times Summary   `json:"times"`
}

// ID names the session: its start, Unix ms.
func (s Session) ID() int64 { return s.Start.UnixMilli() }

// Open tells the session is still going.
func (s Session) Open() bool { return s.End.IsZero() }

// Cost is what went into a session, worth ValueEx Exalted when told.
type Cost struct {
	ValueEx float64 `json:"valueEx"`
}

// Summary is a session's times, in seconds, and how many maps they count.
type Summary struct {
	Active int `json:"active"`
	InMaps int `json:"inMaps"`
	InRest int `json:"inRest"`
	Breaks int `json:"breaks"`
	Maps   int `json:"maps"`
}

func (a Summary) add(b Summary) Summary {
	return Summary{a.Active + b.Active, a.InMaps + b.InMaps, a.InRest + b.InRest, a.Breaks + b.Breaks, a.Maps + b.Maps}
}

func secs(d time.Duration) int { return int(d.Round(time.Second) / time.Second) }

func summary(t Times) Summary {
	return Summary{secs(t.Active), secs(t.InMaps), secs(t.InRest), secs(t.Breaks), len(t.Maps)}
}

// Clock is the session's times up to now (its end, when closed): Times of
// its marks, and the summary with the times carried from before them.
func (s Session) Clock(now time.Time) (Times, Summary) {
	if !s.Open() {
		now = s.End
	}
	t := FarmTimes(s.Marks, time.Time{}, now)
	return t, s.Times.add(summary(t))
}

// Line is a mark read from the game's log as Follow takes it: Open marks
// the game starting, Prev the time of the log's line before (zero: not
// known).
type Line struct {
	Mark
	Open bool
	Prev time.Time
}

// LogPos is where the last read of the game's log stopped (the file, the
// offset of the next line, the time of the last line read).
type LogPos struct {
	Path   string    `json:"path"`
	Offset int64     `json:"offset"`
	Last   time.Time `json:"last,omitzero"`
}

// LogPos returns where the last read of the game's log stopped.
func (s *Store) LogPos() LogPos {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load().Log
}

// Follow files the lines read from the game's log, up to pos, in league's
// open session (one begins with the first line when none is open). Again
// tells the read started over (see overlay.ReadMarks): what is not later
// than the last line read before is had already.
func (s *Store) Follow(leagueName string, lines []Line, pos LogPos, again bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	last := f.Log.Last
	f.Log = pos
	if again {
		lines = slices.DeleteFunc(slices.Clone(lines), func(ln Line) bool { return !ln.At.After(last) })
	}
	if len(lines) == 0 {
		// Only where the read stopped: kept with the next mark (what is
		// read again after a restart till then holds none).
		return nil
	}
	l := s.league(leagueName)
	for _, ln := range lines {
		cur := l.open()
		if ln.Open {
			if cur == nil {
				l.begin(ln.At, nil)
				continue
			}
			prev := ln.Prev
			if prev.IsZero() || prev.Before(cur.Start) {
				prev = cur.lastAt()
			}
			if ln.At.Sub(prev) >= NewSessionAfter {
				l.close(prev)
				l.begin(ln.At, nil)
			} else {
				cur.Marks = append(cur.Marks, Mark{At: prev, Off: true})
			}
			continue
		}
		if cur == nil {
			cur = l.begin(ln.At, nil)
		}
		cur.Marks = append(cur.Marks, ln.Mark)
	}
	return s.save()
}

// open is the league's open session, nil when none.
func (l *league) open() *Session {
	if n := len(l.Sessions); n > 0 && l.Sessions[n-1].Open() {
		return &l.Sessions[n-1]
	}
	return nil
}

// begin opens a session at at, with marks; at is moved on past the last
// session's start, as their starts name them.
func (l *league) begin(at time.Time, marks []Mark) *Session {
	if n := len(l.Sessions); n > 0 && !at.After(l.Sessions[n-1].Start) {
		at = l.Sessions[n-1].Start.Add(time.Millisecond)
	}
	l.Sessions = append(l.Sessions, Session{Start: at, Marks: marks})
	return &l.Sessions[len(l.Sessions)-1]
}

// close ends the open session at end, keeping its times and those of its
// maps left.
func (l *league) close(end time.Time) {
	cur := l.open()
	if cur == nil {
		return
	}
	if end.Before(cur.Start) {
		end = cur.Start
	}
	cur.End = end
	t, sum := cur.Clock(end)
	l.setMapTimes(t.Maps)
	cur.Times, cur.Marks = sum, nil
}

// lastAt is the time of the session's last mark (its start when none).
func (s *Session) lastAt() time.Time {
	if n := len(s.Marks); n > 0 {
		return s.Marks[n-1].At
	}
	return s.Start
}

// where is where the session's marks leave the player: the area (nil: not
// known, or the game closed) and whether they are away.
func (s *Session) where() (*Area, bool) {
	var area *Area
	afk := false
	for _, m := range s.Marks {
		switch {
		case m.Off:
			area, afk = nil, false
		case m.Area != nil:
			area = m.Area
		default:
			afk = m.AFK
		}
	}
	return area, afk
}

// NewSession ends league's open session at at and begins another there,
// from the area the player is in.
func (s *Store) NewSession(leagueName string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.league(leagueName)
	l.restart(at)
	return s.save()
}

func (l *league) restart(at time.Time) {
	var marks []Mark
	if cur := l.open(); cur != nil {
		area, afk := cur.where()
		if area != nil {
			a := *area
			marks = append(marks, Mark{At: at, Area: &a})
			if afk {
				marks = append(marks, Mark{At: at, AFK: true})
			}
		}
		l.close(at)
	}
	l.begin(at, marks)
}

// Sessions returns league's sessions, oldest first.
func (s *Store) Sessions(leagueName string) []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil {
		return nil
	}
	out := slices.Clone(l.Sessions)
	for i := range out {
		out[i].Marks = slices.Clone(out[i].Marks)
		if c := out[i].Cost; c != nil {
			cost := *c
			out[i].Cost = &cost
		}
	}
	return out
}

// session finds league's session id.
func (l *league) session(id int64) int {
	return slices.IndexFunc(l.Sessions, func(s Session) bool { return s.ID() == id })
}

// edit runs fn on league's session id and keeps the change; false when
// there is no such session.
func (s *Store) edit(leagueName string, id int64, fn func(l *league, i int)) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil {
		return false, nil
	}
	i := l.session(id)
	if i < 0 {
		return false, nil
	}
	fn(l, i)
	return true, s.save()
}

// RenameSession names league's session id ("": its number again).
func (s *Store) RenameSession(leagueName string, id int64, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > sessionNameMax {
		name = string(r[:sessionNameMax])
	}
	return s.edit(leagueName, id, func(l *league, i int) { l.Sessions[i].Name = name })
}

// SetSessionCost keeps what went into league's session id (nil, or not
// more than 0: nothing told).
func (s *Store) SetSessionCost(leagueName string, id int64, cost *Cost) (bool, error) {
	if cost != nil && !(cost.ValueEx > 0) {
		cost = nil
	}
	return s.edit(leagueName, id, func(l *league, i int) { l.Sessions[i].Cost = cost })
}

// MergeSession joins league's session id to the one before it: that one
// takes its maps, times and cost, and its end (open, when it was).
func (s *Store) MergeSession(leagueName string, id int64) (bool, error) {
	ok := false
	_, err := s.edit(leagueName, id, func(l *league, i int) {
		if i == 0 {
			return
		}
		ok = true
		prev, cur := &l.Sessions[i-1], l.Sessions[i]
		prev.End, prev.Marks = cur.End, cur.Marks
		prev.Times = prev.Times.add(cur.Times)
		if prev.Name == "" {
			prev.Name = cur.Name
		}
		if cur.Cost != nil {
			c := Cost{ValueEx: cur.Cost.ValueEx}
			if prev.Cost != nil {
				c.ValueEx += prev.Cost.ValueEx
			}
			prev.Cost = &c
		}
		l.Sessions = slices.Delete(l.Sessions, i, i+1)
	})
	return ok, err
}

// DeleteSession forgets league's session id and its maps. The open one
// begins again at now, from where the player is.
func (s *Store) DeleteSession(leagueName string, id int64, now time.Time) (bool, error) {
	return s.edit(leagueName, id, func(l *league, i int) {
		from, to := l.Sessions[i].Start, time.Time{}
		if i+1 < len(l.Sessions) {
			to = l.Sessions[i+1].Start
		}
		l.Maps = slices.DeleteFunc(l.Maps, func(m Map) bool {
			return !m.At.Before(from) && (to.IsZero() || m.At.Before(to))
		})
		if !l.Sessions[i].Open() {
			l.Sessions = slices.Delete(l.Sessions, i, i+1)
			return
		}
		l.restart(now)
		// The session just closed goes with its maps.
		l.Sessions = slices.Delete(l.Sessions, i, i+1)
	})
}

// Group puts maps (newest first, as Maps returns them) in their sessions
// (oldest first): bySession[i] are session i's maps, newest first; before
// are those first entered before the first session.
func Group(sessions []Session, maps []Map) (bySession [][]Map, before []Map) {
	bySession = make([][]Map, len(sessions))
	for _, m := range maps {
		i := len(sessions) - 1
		for i >= 0 && m.At.Before(sessions[i].Start) {
			i--
		}
		if i < 0 {
			before = append(before, m)
			continue
		}
		bySession[i] = append(bySession[i], m)
	}
	return bySession, before
}
