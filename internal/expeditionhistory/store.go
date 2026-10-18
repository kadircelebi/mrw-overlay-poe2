// Package expeditionhistory keeps the Expedition rewards the player chose on
// the Runeshape Combinations panel, every choice with the map it was made
// in. Nothing leaves the PC.
package expeditionhistory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

const fileVersion = 1

// Entry is one chosen reward. Area is the map it was chosen in (id, level and
// seed: one instance), Name the currency item it grants in English ("" for a
// reward that is none), Display the name as the game's language writes it,
// Text the line as read. ValueEx is its worth when chosen (the unit's when
// the count is unknown). Done is set once the player went on to another map.
type Entry struct {
	At         time.Time `json:"at"`
	AreaID     string    `json:"area"`
	AreaLevel  int       `json:"level"`
	Seed       string    `json:"seed,omitempty"`
	Name       string    `json:"name"`
	Display    string    `json:"display,omitempty"`
	Text       string    `json:"text"`
	Count      int       `json:"count"`
	CountKnown bool      `json:"countKnown"`
	ValueEx    float64   `json:"valueEx"`
	Done       bool      `json:"done"`
}

type file struct {
	Version int `json:"version"`
	// Leagues holds each league's entries, newest first.
	Leagues map[string][]Entry `json:"leagues"`
}

// Store is the local file of chosen rewards.
type Store struct {
	mu   sync.Mutex
	path string
	data *file
}

// New opens the store at dataDir/data/expedition_history.json; the file is
// read on first use.
func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "data", "expedition_history.json")}
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
		f.Leagues = map[string][]Entry{}
	}
	f.Version = fileVersion
	s.data = f
	return f
}

// Entries returns league's entries, newest first.
func (s *Store) Entries(league string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.load().Leagues[league])
}

// Pending is the latest reward chosen in the map the player is still in, if
// any.
func (s *Store) Pending(league string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.load().Leagues[league]
	if len(list) > 0 && !list[0].Done {
		return list[0], true
	}
	return Entry{}, false
}

// Choose keeps e. A map can give several rewards, so every choice is kept;
// a choice in another map closes the ones before.
func (s *Store) Choose(league string, e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	list := f.Leagues[league]
	closeOthers(list, e)
	e.Done = false
	f.Leagues[league] = append([]Entry{e}, list...)
	return s.save(f)
}

// Entered follows the player into the area id with seed: another map than
// the pending rewards' closes them. Hideouts and towns are only a trip home
// and close nothing (rest). It reports whether anything changed.
func (s *Store) Entered(league, id, seed string, rest bool) (bool, error) {
	if rest {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	if !closeOthers(f.Leagues[league], Entry{AreaID: id, Seed: seed}) {
		return false, nil
	}
	return true, s.save(f)
}

// closeOthers marks done the open entries of maps other than here's (open
// ones are the newest, so it stops at the first done one) and reports
// whether it closed any.
func closeOthers(list []Entry, here Entry) bool {
	closed := false
	for i := range list {
		if list[i].Done {
			break
		}
		if !sameMap(list[i], here) {
			list[i].Done = true
			closed = true
		}
	}
	return closed
}

// Undo forgets the latest choice of the map the player is in and returns
// it; a finished map's choices are not undone.
func (s *Store) Undo(league string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	list := f.Leagues[league]
	if len(list) == 0 || list[0].Done {
		return Entry{}, false, nil
	}
	gone := list[0]
	f.Leagues[league] = slices.Clone(list[1:])
	return gone, true, s.save(f)
}

// ResetMap forgets every choice of the map the player is in, for choosing
// again from the start, and returns how many there were.
func (s *Store) ResetMap(league string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	list := f.Leagues[league]
	n := 0
	for n < len(list) && !list[n].Done {
		n++
	}
	if n == 0 {
		return 0, nil
	}
	f.Leagues[league] = slices.Clone(list[n:])
	return n, s.save(f)
}

// Delete forgets the choice made at at, from any map.
func (s *Store) Delete(league string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	list := f.Leagues[league]
	i := slices.IndexFunc(list, func(e Entry) bool { return e.At.Equal(at) })
	if i < 0 {
		return false, nil
	}
	f.Leagues[league] = slices.Delete(slices.Clone(list), i, i+1)
	return true, s.save(f)
}

func sameMap(a, b Entry) bool { return a.AreaID == b.AreaID && a.Seed == b.Seed }

func (s *Store) save(f *file) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return prices.WriteFileAtomic(s.path, raw)
}

// Clear forgets every league's entries.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = nil
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Total is a sum of chosen rewards, one per map.
type Total struct {
	Maps    int     `json:"maps"`
	ValueEx float64 `json:"valueEx"`
}

// Open adds up the rewards chosen in the map the player is still in.
func Open(entries []Entry) Total {
	var t Total
	for _, e := range entries {
		if e.Done {
			break
		}
		t.Maps++
		t.ValueEx += e.ValueEx
	}
	return t
}

// Sum adds up the rewards chosen at or after since (zero: all), the one of
// the map the player is still in included.
func Sum(entries []Entry, since time.Time) Total {
	var t Total
	for _, e := range entries {
		if e.At.Before(since) {
			continue
		}
		t.Maps++
		t.ValueEx += e.ValueEx
	}
	return t
}
