// Package lootlog keeps what each map brought: the currency in the
// inventory read before a map and again after it, map by map. The game logs
// the areas the player goes to; a map is the first area entered from a
// hideout or town, the areas entered from it without going back (a boss
// arena, the Abyss below) are part of it, and going back into the same
// instance (same area and seed) goes on with it. Nothing leaves the PC.
package lootlog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

const fileVersion = 1

// Scan is one read of the inventory: how many of each listed item it held
// (by English name) and how many cells were not read for sure.
type Scan struct {
	At     time.Time      `json:"at"`
	Items  map[string]int `json:"items"`
	Unsure int            `json:"unsure,omitempty"`
	Doubts []Doubt        `json:"doubts,omitempty"`
	// Exp is the character's total experience, Next the total the next level
	// takes and Gold the gold under the inventory, when read.
	Exp  *int64 `json:"exp,omitempty"`
	Next *int64 `json:"next,omitempty"`
	Gold *int64 `json:"gold,omitempty"`
	// visit is the hideout or town visit the read was made in (in memory
	// only: a read before the app started is from another visit).
	visit int
	// live tells the read's experience and gold are from since the app
	// started (in memory only: maps may have been played before that).
	live bool
}

// Doubt is a cell a read did not read for sure: its row and column (from
// the top left, from 1), the item taken for it and its count, and whether
// the item (Why "item": which of the items sharing its icon) or the count
// (Why "count": a figure may be missing) is in doubt. After marks a run's
// doubts from the read after the map.
type Doubt struct {
	Row   int    `json:"row"`
	Col   int    `json:"col"`
	Name  string `json:"name,omitempty"`
	Count int    `json:"count"`
	Why   string `json:"why"`
	After bool   `json:"after,omitempty"`
}

// Item is one item a run brought: how many more there were after it than
// before, and one's worth when the run was read.
type Item struct {
	Name   string  `json:"name"`
	Count  int     `json:"count"`
	EachEx float64 `json:"eachEx"`
}

// Run is one trip into a map: from the last read in the hideout before it
// to the first read there after it. Fresh tells the read before was made
// on that same visit to the hideout (an older one may miss what was put in
// the stash since, and the gain then shows short); NoBase that there was
// none before, Unread that none was made after (the player went on to the
// next map): no gain is known then.
type Run struct {
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Items   []Item    `json:"items,omitempty"`
	ValueEx float64   `json:"valueEx"`
	Fresh   bool      `json:"fresh"`
	NoBase  bool      `json:"noBase,omitempty"`
	Unread  bool      `json:"unread,omitempty"`
	Unsure  int       `json:"unsure,omitempty"`
	Doubts  []Doubt   `json:"doubts,omitempty"`
	// Exp and Gold are the experience and gold after the run less before it,
	// when both reads have them. A death's experience loss and gold spent
	// between the reads count against them, so either may be negative.
	Exp  *int64 `json:"exp,omitempty"`
	Gold *int64 `json:"gold,omitempty"`
}

// Map is one map instance and its runs, newest last.
type Map struct {
	At      time.Time `json:"at"`
	AreaID  string    `json:"area"`
	Level   int       `json:"level"`
	Seed    string    `json:"seed,omitempty"`
	Runs    []Run     `json:"runs"`
	ValueEx float64   `json:"valueEx"`
	// Exp and Gold add up its runs' (nil: no run has one).
	Exp  *int64 `json:"exp,omitempty"`
	Gold *int64 `json:"gold,omitempty"`
	// Secs is the time spent in it, from the game's log (0: not known).
	Secs int `json:"secs,omitempty"`
}

// Area is an area the game built (overlay.Area's fields).
type Area struct {
	ID    string `json:"id"`
	Level int    `json:"level,omitempty"`
	Seed  string `json:"seed,omitempty"`
	Rest  bool   `json:"rest,omitempty"` // hideout or town
}

// trip is the map the player went into from the hideout, with the read
// they left with; Back is set once they are home again, waiting for the
// read that closes the run.
type trip struct {
	AreaID  string    `json:"area"`
	Level   int       `json:"level"`
	Seed    string    `json:"seed,omitempty"`
	Entered time.Time `json:"entered"`
	Base    *Scan     `json:"base,omitempty"`
	Fresh   bool      `json:"fresh"`
	Back    bool      `json:"back"`
	BackAt  time.Time `json:"backAt,omitempty"`
}

type league struct {
	// Maps are newest first.
	Maps []Map `json:"maps"`
	// Last is the latest read made in a hideout or town.
	Last *Scan `json:"last,omitempty"`
	Trip *trip `json:"trip,omitempty"`
	// Sessions are oldest first (see Session).
	Sessions []Session `json:"sessions,omitempty"`
}

type file struct {
	Version int                `json:"version"`
	Leagues map[string]*league `json:"leagues"`
	// Log is where the last read of the game's log stopped.
	Log LogPos `json:"log"`
}

// Store is the local file of map runs.
type Store struct {
	mu   sync.Mutex
	path string
	data *file
	// Where the player is, as the game's log said since the app started.
	known  bool
	inRest bool
	visit  int
}

// New opens the store at dataDir/data/loot_log.json; the file is read on
// first use.
func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "data", "loot_log.json")}
}

func (s *Store) load() *file {
	if s.data != nil {
		return s.data
	}
	f := &file{}
	if raw, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(raw, f)
	}
	if f.Leagues == nil {
		f.Leagues = map[string]*league{}
	}
	f.Version = fileVersion
	s.data = f
	return f
}

func (s *Store) league(name string) *league {
	f := s.load()
	l := f.Leagues[name]
	if l == nil {
		l = &league{}
		f.Leagues[name] = l
	}
	return l
}

// Known tells whether the store knows where the player is; Place tells it
// once (the app starts while the game runs): the area the log names last.
func (s *Store) Known() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.known
}

// Place sets where the player is without it being a move (no map begins).
func (s *Store) Place(a Area) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.known {
		return
	}
	s.known, s.inRest = true, a.Rest
	if a.Rest {
		s.visit++
	}
}

// InRest tells whether the player is in a hideout or town.
func (s *Store) InRest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.known && s.inRest
}

// Entered follows the player into an area at now.
func (s *Store) Entered(leagueName string, a Area, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wasRest, wasKnown := s.inRest, s.known
	s.known, s.inRest = true, a.Rest
	l := s.league(leagueName)
	if a.Rest {
		s.visit++
		if l.Trip != nil && !l.Trip.Back && wasKnown && !wasRest {
			l.Trip.Back, l.Trip.BackAt = true, now
			return s.save()
		}
		return nil
	}
	if wasKnown && !wasRest {
		return nil // an area entered from the map: part of it
	}
	if l.Trip != nil && l.Trip.Back {
		// Home and out again without a read: that run is not known, and the
		// last read's experience and gold are from before it.
		s.closeRun(l, nil, now)
		if l.Last != nil {
			l.Last.Exp, l.Last.Next, l.Last.Gold = nil, nil, nil
		}
	}
	t := &trip{AreaID: a.ID, Level: a.Level, Seed: a.Seed, Entered: now}
	if l.Last != nil {
		base := *l.Last
		if !base.live {
			base.Exp, base.Next, base.Gold = nil, nil, nil
		}
		t.Base = &base
		t.Fresh = l.Last.visit == s.visit && s.visit > 0
	}
	l.Trip = t
	return s.save()
}

// Scanned keeps a read of the inventory. Made in a hideout or town, it is
// the next map's base; the first one after a map also closes that map's
// run, worth each item at worth(name) (Exalted, 0 when unpriced), and is
// returned with the map. A read made elsewhere keeps nothing.
func (s *Store) Scanned(leagueName string, sc Scan, worth func(string) float64) (*Map, *Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.known || !s.inRest {
		return nil, nil, nil
	}
	l := s.league(leagueName)
	sc.visit = s.visit
	var m *Map
	var r *Run
	keep := sc
	if l.Trip != nil && l.Trip.Back {
		m, r = s.closeRun(l, &sc, sc.At, worth)
	} else if l.Last != nil && l.Last.live {
		// No map since the last read: experience has not moved, and a read
		// that could not tell the gold keeps the last one.
		if keep.Exp == nil {
			keep.Exp, keep.Next = l.Last.Exp, l.Last.Next
		}
		if keep.Gold == nil {
			keep.Gold = l.Last.Gold
		}
	}
	keep.live = true
	keep.Items = maps(sc.Items)
	l.Last = &keep
	return m, r, s.save()
}

// NeedExp tells whether the character's experience should be read now: back
// from a map, or no read in a hideout or town since the app started has it.
// Experience moves only in a map, so once known it is not read again.
func (s *Store) NeedExp(leagueName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil || l.Last == nil || (l.Trip != nil && l.Trip.Back) {
		return true
	}
	return l.Last.Exp == nil || !l.Last.live
}

// ExpLeft is the experience the next level still takes, from the latest read
// that has it.
func (s *Store) ExpLeft(leagueName string) *int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil || l.Last == nil || l.Last.Exp == nil || l.Last.Next == nil {
		return nil
	}
	return difference(l.Last.Exp, l.Last.Next)
}

// SetReadout keeps experience (with the total the next level takes) and gold
// read in a hideout or town outside a read of the inventory (nil: not read)
// on the latest read, for the next map to begin from.
func (s *Store) SetReadout(leagueName string, exp, next, gold *int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.known || !s.inRest {
		return nil
	}
	l := s.load().Leagues[leagueName]
	if l == nil || l.Last == nil || (l.Trip != nil && l.Trip.Back) {
		return nil
	}
	changed := false
	if !l.Last.live && (exp != nil || gold != nil) {
		l.Last.Exp, l.Last.Next, l.Last.Gold, l.Last.live = nil, nil, nil, true
		changed = true
	}
	if exp != nil && (l.Last.Exp == nil || *l.Last.Exp != *exp) {
		l.Last.Exp, l.Last.Next, changed = exp, next, true
	}
	if gold != nil && (l.Last.Gold == nil || *l.Last.Gold != *gold) {
		l.Last.Gold, changed = gold, true
	}
	if !changed {
		return nil
	}
	return s.save()
}

// difference is after less before, when both are known.
func difference(before, after *int64) *int64 {
	if before == nil || after == nil {
		return nil
	}
	d := *after - *before
	return &d
}

// addKnown adds a known b to a, either maybe unknown.
func addKnown(a, b *int64) *int64 {
	switch {
	case b == nil:
		return a
	case a == nil:
		n := *b
		return &n
	}
	n := *a + *b
	return &n
}

// closeRun ends the trip's run with read after (nil: none was made) and
// files it under its map.
func (s *Store) closeRun(l *league, after *Scan, at time.Time, worth ...func(string) float64) (*Map, *Run) {
	t := l.Trip
	l.Trip = nil
	r := Run{To: at, Fresh: t.Fresh}
	switch {
	case after == nil:
		r.Unread = true
	case t.Base == nil:
		r.NoBase = true
		r.Unsure = after.Unsure
		r.Doubts = afterDoubts(after.Doubts)
	default:
		r.From = t.Base.At
		r.Unsure = t.Base.Unsure + after.Unsure
		r.Doubts = append(append([]Doubt(nil), t.Base.Doubts...), afterDoubts(after.Doubts)...)
		r.Exp, r.Gold = difference(t.Base.Exp, after.Exp), difference(t.Base.Gold, after.Gold)
		r.Items = Gain(t.Base.Items, after.Items)
		for i := range r.Items {
			if len(worth) > 0 && worth[0] != nil {
				r.Items[i].EachEx = worth[0](r.Items[i].Name)
			}
			r.ValueEx += r.Items[i].EachEx * float64(r.Items[i].Count)
		}
	}
	if r.From.IsZero() {
		r.From = t.Entered
	}
	if len(l.Maps) > 0 && l.Maps[0].AreaID == t.AreaID && l.Maps[0].Seed == t.Seed {
		l.Maps[0].Runs = append(l.Maps[0].Runs, r)
		l.Maps[0].ValueEx += r.ValueEx
		l.Maps[0].Exp, l.Maps[0].Gold = addKnown(l.Maps[0].Exp, r.Exp), addKnown(l.Maps[0].Gold, r.Gold)
	} else {
		l.Maps = append([]Map{{At: t.Entered, AreaID: t.AreaID, Level: t.Level, Seed: t.Seed, Runs: []Run{r}, ValueEx: r.ValueEx, Exp: addKnown(nil, r.Exp), Gold: addKnown(nil, r.Gold)}}, l.Maps...)
	}
	m := l.Maps[0]
	return &m, &r
}

func afterDoubts(d []Doubt) []Doubt {
	out := make([]Doubt, len(d))
	for i, x := range d {
		x.After = true
		out[i] = x
	}
	return out
}

// Gain is what after holds more of than before, item by item, most first:
// Σ max(0, after − before). What went down (into the stash, used) counts
// for nothing.
func Gain(before, after map[string]int) []Item {
	var out []Item
	for name, n := range after {
		if d := n - before[name]; d > 0 {
			out = append(out, Item{Name: name, Count: d})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func maps(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Waiting is the map whose run waits for a read in the hideout, if any,
// and since when the player is back.
func (s *Store) Waiting(leagueName string) (areaID string, since time.Time, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil || l.Trip == nil || !l.Trip.Back {
		return "", time.Time{}, false
	}
	return l.Trip.AreaID, l.Trip.BackAt, true
}

// State is where the log stands, as the box over the game shows it: where
// the player is, the latest read in a hideout or town (Fresh: made on this
// visit there) and the map gone into from one, with the read it began
// from (TripFresh as Fresh, then).
type State struct {
	Known, InRest bool
	Last          *Scan
	Fresh         bool
	TripArea      string
	Entered       time.Time
	Base          *Scan
	TripFresh     bool
	Back          bool
	BackAt        time.Time
}

// State returns league's state.
func (s *Store) State(leagueName string) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := State{Known: s.known, InRest: s.known && s.inRest}
	l := s.load().Leagues[leagueName]
	if l == nil {
		return st
	}
	if l.Last != nil {
		last := *l.Last
		st.Last = &last
		st.Fresh = l.Last.visit == s.visit && s.visit > 0
	}
	if t := l.Trip; t != nil {
		st.TripArea, st.Entered, st.TripFresh, st.Back, st.BackAt = t.AreaID, t.Entered, t.Fresh, t.Back, t.BackAt
		if t.Base != nil {
			base := *t.Base
			st.Base = &base
		}
	}
	return st
}

// SetMapTimes keeps the time spent in league's maps that times names (the
// same instance, first entered within a couple of minutes of the map's
// start), and reports whether one changed.
func (s *Store) SetMapTimes(leagueName string, times []MapTime) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil || !l.setMapTimes(times) {
		return false, nil
	}
	return true, s.save()
}

func (l *league) setMapTimes(times []MapTime) bool {
	changed := false
	for _, mt := range times {
		secs := int(mt.Dur.Round(time.Second) / time.Second)
		for i := range l.Maps {
			m := &l.Maps[i]
			if m.AreaID != mt.AreaID || m.Seed != mt.Seed || m.At.Sub(mt.At).Abs() > 2*time.Minute {
				continue
			}
			if m.Secs != secs {
				m.Secs, changed = secs, true
			}
			break
		}
	}
	return changed
}

// Maps returns league's maps, newest first.
func (s *Store) Maps(leagueName string) []Map {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil {
		return nil
	}
	return slices.Clone(l.Maps)
}

// Delete forgets the map first entered at at.
func (s *Store) Delete(leagueName string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.load().Leagues[leagueName]
	if l == nil {
		return nil
	}
	l.Maps = slices.DeleteFunc(l.Maps, func(m Map) bool { return m.At.Equal(at) })
	return s.save()
}

// Clear forgets every league's maps and reads.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = nil
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) save() error {
	raw, err := json.Marshal(s.load())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return prices.WriteFileAtomic(s.path, raw)
}

// Total is a sum of maps.
type Total struct {
	Maps    int     `json:"maps"`
	ValueEx float64 `json:"valueEx"`
	// Exp and Gold add up the maps that have them; ExpMaps and GoldMaps count
	// those maps (for an average that leaves out the unread ones).
	Exp      int64 `json:"exp,omitempty"`
	Gold     int64 `json:"gold,omitempty"`
	ExpMaps  int   `json:"expMaps,omitempty"`
	GoldMaps int   `json:"goldMaps,omitempty"`
}

// Sum adds up the maps first entered at or after since (zero: all).
func Sum(list []Map, since time.Time) Total {
	var t Total
	for _, m := range list {
		if m.At.Before(since) {
			continue
		}
		t.Maps++
		t.ValueEx += m.ValueEx
		if m.Exp != nil {
			t.Exp += *m.Exp
			t.ExpMaps++
		}
		if m.Gold != nil {
			t.Gold += *m.Gold
			t.GoldMaps++
		}
	}
	return t
}
