// Package tradehistory keeps the player's merchant sales. The trade site
// lists only the latest ones, so every fetch is merged into a local file and
// the league's history grows from the first fetch on. Nothing leaves the PC.
package tradehistory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
)

const fileVersion = 1

type file struct {
	Version int `json:"version"`
	// Leagues holds each league's sales, newest first.
	Leagues map[string][]trade.HistorySale `json:"leagues"`
	// Fetched is when each league was last fetched.
	Fetched map[string]time.Time `json:"fetched,omitempty"`
}

// Store is the local sales file.
type Store struct {
	mu   sync.Mutex
	path string
	data *file
}

// New opens the store at dataDir/data/trade_history.json; the file is read
// on first use.
func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "data", "trade_history.json")}
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
		f.Leagues = map[string][]trade.HistorySale{}
	}
	if f.Fetched == nil {
		f.Fetched = map[string]time.Time{}
	}
	f.Version = fileVersion
	s.data = f
	return f
}

// Sales returns league's sales, newest first.
func (s *Store) Sales(league string) []trade.HistorySale {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.load().Leagues[league])
}

// Fetched is when league was last fetched (zero before the first time).
func (s *Store) Fetched(league string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load().Fetched[league]
}

// Merge adds the sales not yet kept to league and saves the file. It
// returns the number added.
func (s *Store) Merge(league string, sales []trade.HistorySale, fetched time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	merged, added := merge(f.Leagues[league], sales)
	f.Leagues[league] = merged
	f.Fetched[league] = fetched
	return added, s.save(f)
}

// merge joins fresh sales into kept ones. A sale is the same sale when its
// item and time match: an item sold, bought back and sold again is two.
func merge(kept, fresh []trade.HistorySale) ([]trade.HistorySale, int) {
	type key struct {
		id string
		at int64
	}
	seen := make(map[key]bool, len(kept))
	for _, s := range kept {
		seen[key{s.ItemID, s.Time.Unix()}] = true
	}
	out := slices.Clone(kept)
	added := 0
	for _, s := range fresh {
		k := key{s.ItemID, s.Time.Unix()}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
		added++
	}
	slices.SortStableFunc(out, func(a, b trade.HistorySale) int { return b.Time.Compare(a.Time) })
	return out, added
}

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

// Clear forgets every league's sales.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = nil
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
