package trade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

// Classes whose normal socket maximum is 2 (exceptional = 3+) and 1 (exceptional = 2+).
var (
	threeSocketClasses = map[string]bool{
		"Body Armours": true, "Bows": true, "Crossbows": true, "Quarterstaves": true,
		"Staves": true, "Talismans": true, "Two Hand Maces": true,
	}
	twoSocketClasses = map[string]bool{
		"Boots": true, "Bucklers": true, "Foci": true, "Gloves": true, "Helmets": true,
		"One Hand Maces": true, "Sceptres": true, "Shields": true, "Spears": true, "Wands": true,
	}
)

// ExceptionalQualityMin is the lowest quality a base can only drop with as exceptional.
const ExceptionalQualityMin = 21

// ExceptionalSocketMin returns the socket count that makes a base of the given
// class exceptional, or 0 when the class has no exceptional socket variant.
func ExceptionalSocketMin(class string) int {
	switch {
	case threeSocketClasses[class]:
		return 3
	case twoSocketClasses[class]:
		return 2
	}
	return 0
}

// Candidate is a base type the scanner should price.
type Candidate struct {
	Base     string
	Priority int // lower scans first
}

type keyState struct {
	prices.ExceptionalPrice
	LastAttempt time.Time `json:"last_attempt"`
	LastError   string    `json:"last_error,omitempty"`
}

type scanState struct {
	Version int                  `json:"version"`
	League  string               `json:"league"`
	Classes map[string]string    `json:"classes"`
	Keys    map[string]*keyState `json:"keys"`
}

const scanStateVersion = 1

// Scanner prices exceptional bases in the background.
type Scanner struct {
	client    *Client
	statePath string
	log       func(string)

	mu         sync.Mutex
	st         scanState
	candidates []Candidate
	market     *prices.Snapshot
	hotEx      float64
	current    string        // key waiting for / in its search
	last       string        // last finished key with its result
	wake       chan struct{} // nudges Run when market or candidates arrive
	onChange   func()        // called after each scan (outside the lock)
}

// SetOnChange registers a callback run after every scan attempt.
func (s *Scanner) SetOnChange(f func()) {
	s.mu.Lock()
	s.onChange = f
	s.mu.Unlock()
}

// NewScanner loads (or starts) the persistent scan state.
func NewScanner(client *Client, statePath string, log func(string)) *Scanner {
	s := &Scanner{client: client, statePath: statePath, log: log, hotEx: 25, wake: make(chan struct{}, 1)}
	s.st = scanState{Version: scanStateVersion, League: client.league,
		Classes: map[string]string{}, Keys: map[string]*keyState{}}
	if data, err := os.ReadFile(statePath); err == nil {
		var loaded scanState
		if json.Unmarshal(data, &loaded) == nil && loaded.Version == scanStateVersion && loaded.League == client.league {
			if loaded.Classes == nil {
				loaded.Classes = map[string]string{}
			}
			if loaded.Keys == nil {
				loaded.Keys = map[string]*keyState{}
			}
			s.st = loaded
		}
	}
	return s
}

func (s *Scanner) logf(format string, args ...any) {
	if s.log != nil {
		s.log(fmt.Sprintf(format, args...))
	}
}

// SetCandidates replaces the list of bases to scan.
func (s *Scanner) SetCandidates(c []Candidate) {
	s.mu.Lock()
	s.candidates = c
	s.mu.Unlock()
	s.nudge()
}

func (s *Scanner) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// SetMarket provides exchange rates for converting listing prices.
func (s *Scanner) SetMarket(m *prices.Snapshot) {
	s.mu.Lock()
	s.market = m
	s.mu.Unlock()
	s.nudge()
}

// SetBudget changes the share of the trade rate limit the scanner may use.
func (s *Scanner) SetBudget(b float64) {
	s.client.Search.SetBudget(b)
	s.client.Fetch.SetBudget(b)
}

// SetHotThreshold sets the value (in Exalted) above which a base is rescanned often.
func (s *Scanner) SetHotThreshold(ex float64) {
	s.mu.Lock()
	if ex > 0 {
		s.hotEx = ex
	}
	s.mu.Unlock()
}

func keyOf(base string, kind prices.ExceptionalKind) string { return base + "|" + string(kind) }

type target struct {
	base     string
	kind     prices.ExceptionalKind
	min      int
	priority int
	state    *keyState
}

// refreshAfter returns how long a result stays fresh.
func (s *Scanner) refreshAfter(k *keyState) time.Duration {
	switch {
	case k.LastError != "":
		return time.Hour
	case k.Listings == 0:
		return 72 * time.Hour
	case k.ValueEx >= s.hotEx:
		return 6 * time.Hour
	default:
		return 48 * time.Hour
	}
}

// next picks the most urgent due target (locked).
func (s *Scanner) next(now time.Time) *target {
	var due []target
	for _, c := range s.candidates {
		class := s.st.Classes[c.Base]
		kinds := []struct {
			kind prices.ExceptionalKind
			min  int
		}{{prices.KindQuality, ExceptionalQualityMin}}
		switch {
		case class == "":
			kinds = append(kinds, struct {
				kind prices.ExceptionalKind
				min  int
			}{prices.KindSockets, 2}) // probe; the class is learned from results
		case ExceptionalSocketMin(class) > 0:
			kinds = append(kinds, struct {
				kind prices.ExceptionalKind
				min  int
			}{prices.KindSockets, ExceptionalSocketMin(class)})
		}
		for _, k := range kinds {
			ks := s.st.Keys[keyOf(c.Base, k.kind)]
			if ks != nil && ks.Min == k.min && now.Sub(ks.LastAttempt) < s.refreshAfter(ks) {
				continue
			}
			due = append(due, target{base: c.Base, kind: k.kind, min: k.min, priority: c.Priority, state: ks})
		}
	}
	if len(due) == 0 {
		return nil
	}
	sort.SliceStable(due, func(i, j int) bool {
		a, b := due[i], due[j]
		if (a.state == nil) != (b.state == nil) {
			return a.state == nil // never scanned first
		}
		if a.priority != b.priority {
			return a.priority < b.priority
		}
		if a.state != nil && b.state != nil {
			return a.state.LastAttempt.Before(b.state.LastAttempt)
		}
		return a.base < b.base
	})
	return &due[0]
}

// Run scans until ctx is cancelled.
func (s *Scanner) Run(ctx context.Context) {
	for {
		s.mu.Lock()
		ready := s.market != nil && len(s.candidates) > 0
		var t *target
		if ready {
			t = s.next(time.Now())
		}
		s.mu.Unlock()

		if t == nil {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			case <-time.After(5 * time.Minute):
			}
			continue
		}

		s.mu.Lock()
		s.current = keyLabel(t.base, t.kind, t.min)
		notify := s.onChange
		s.mu.Unlock()
		if notify != nil {
			notify() // show what is queued and when it runs
		}

		err := s.scan(ctx, t)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			s.logf("[Tarama] %s %s: %v", t.base, t.kind, err)
		}
		s.mu.Lock()
		s.last = s.current
		switch ks := s.st.Keys[keyOf(t.base, t.kind)]; {
		case err != nil:
			s.last += " — hata"
		case ks == nil:
			s.last += " — sınıf öğrenildi"
		case ks.Listings == 0:
			s.last += " — ilan yok"
		case ks.Samples == 0:
			s.last += fmt.Sprintf(" — %d ilan", ks.Listings)
		default:
			s.last += fmt.Sprintf(" — %.0f ex", ks.ValueEx)
		}
		s.current = ""
		notify = s.onChange
		s.mu.Unlock()
		s.save()
		if notify != nil {
			notify()
		}
	}
}

func (s *Scanner) scan(ctx context.Context, t *target) error {
	q := NewBaseQuery(t.base, "normal")
	if t.kind == prices.KindQuality {
		q.SetMin("type_filters", "quality", t.min)
	} else {
		q.SetMin("equipment_filters", "rune_sockets", t.min)
	}

	record := func(p prices.ExceptionalPrice, err error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		ks := &keyState{ExceptionalPrice: p, LastAttempt: time.Now().UTC()}
		if err != nil {
			// Keep the last good price; only note the failure.
			if old := s.st.Keys[keyOf(t.base, t.kind)]; old != nil {
				ks.ExceptionalPrice = old.ExceptionalPrice
			} else {
				ks.ExceptionalPrice = prices.ExceptionalPrice{Base: t.base, Kind: t.kind, Min: t.min}
			}
			ks.LastError = err.Error()
		}
		s.st.Keys[keyOf(t.base, t.kind)] = ks
	}

	res, err := s.client.RunSearch(ctx, q)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			record(prices.ExceptionalPrice{Base: t.base, Kind: t.kind, Min: t.min}, err)
		}
		return err
	}
	// Timestamp the result when the search actually ran, not when it was queued.
	price := prices.ExceptionalPrice{Base: t.base, Kind: t.kind, Min: t.min, Listings: res.Total, ScannedAt: time.Now().UTC()}
	if res.Total == 0 || len(res.Result) == 0 {
		record(price, nil)
		return nil
	}

	listings, err := s.client.FetchListings(ctx, res.ID, res.Result)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			record(price, err)
		}
		return err
	}

	s.mu.Lock()
	market := s.market
	class := s.st.Classes[t.base]
	for _, l := range listings {
		if l.Class != "" {
			class = l.Class
			s.st.Classes[t.base] = class
			break
		}
	}
	s.mu.Unlock()
	price.Class = class

	// A socket probe on a class that normally has 2 sockets found ordinary
	// items; drop it and let the next pass search with the right minimum.
	if t.kind == prices.KindSockets && ExceptionalSocketMin(class) != t.min {
		s.mu.Lock()
		delete(s.st.Keys, keyOf(t.base, t.kind))
		s.mu.Unlock()
		return nil
	}

	var values []float64
	for _, l := range listings {
		if l.Rarity != "Normal" {
			continue
		}
		if t.kind == prices.KindSockets && l.Sockets < t.min {
			continue
		}
		if t.kind == prices.KindQuality && l.Quality < t.min {
			continue
		}
		if ex, ok := market.ToEx(l.Amount, l.Currency); ok && ex > 0 {
			values = append(values, ex)
		}
	}
	price.ValueEx, price.Samples = TrimmedMean(values)
	record(price, nil)
	return nil
}

// TrimmedMean averages the cheapest listings after dropping the lowest one or
// two, which are often mispriced or subtly different items.
func TrimmedMean(values []float64) (float64, int) {
	if len(values) == 0 {
		return 0, 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	switch {
	case len(v) >= 8:
		v = v[2:]
	case len(v) >= 5:
		v = v[1:]
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v)), len(v)
}

func (s *Scanner) save() {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.st, "", " ")
	s.mu.Unlock()
	if err == nil {
		if err := prices.WriteFileAtomic(s.statePath, data); err != nil {
			s.logf("[Tarama] durum kaydedilemedi: %v", err)
		}
	}
}

// Results returns every successfully scanned exceptional price.
func (s *Scanner) Results() []prices.ExceptionalPrice {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []prices.ExceptionalPrice
	for _, k := range s.st.Keys {
		if !k.ScannedAt.IsZero() {
			out = append(out, k.ExceptionalPrice)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Base != out[j].Base {
			return out[i].Base < out[j].Base
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func keyLabel(base string, kind prices.ExceptionalKind, min int) string {
	if kind == prices.KindQuality {
		return fmt.Sprintf("%s · %%%d+ kalite", base, min)
	}
	return fmt.Sprintf("%s · %d soket", base, min)
}

// Status summarises scanner progress for the UI.
type Status struct {
	Candidates int     `json:"candidates"`
	Keys       int     `json:"keys"`
	Scanned    int     `json:"scanned"`
	Valuable   int     `json:"valuable"`
	Current    string  `json:"current"`
	Last       string  `json:"last"`
	NextAt     int64   `json:"next_at"` // unix ms of the next search slot
	EtaSec     float64 `json:"eta_sec"` // time to scan everything not yet scanned
}

// Status returns current progress.
func (s *Scanner) Status() Status {
	next := s.client.Search.NextIn()
	spacing := s.client.Search.Spacing()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Candidates: len(s.candidates), Current: s.current, Last: s.last,
		NextAt: time.Now().Add(next).UnixMilli()}
	for _, c := range s.candidates {
		st.Keys++ // quality
		if class := s.st.Classes[c.Base]; class == "" || ExceptionalSocketMin(class) > 0 {
			st.Keys++ // sockets
		}
	}
	for _, k := range s.st.Keys {
		if !k.ScannedAt.IsZero() {
			st.Scanned++
			if k.ValueEx >= s.hotEx {
				st.Valuable++
			}
		}
	}
	if remaining := st.Keys - st.Scanned; remaining > 0 {
		st.EtaSec = float64(remaining) * spacing.Seconds()
	}
	return st
}

// SocketClasses returns the item classes whose exceptional socket minimum is min.
func SocketClasses(min int) []string {
	src := twoSocketClasses
	if min == 3 {
		src = threeSocketClasses
	}
	out := make([]string, 0, len(src))
	for c := range src {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
