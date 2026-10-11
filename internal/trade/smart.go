package trade

import (
	"math"
	"strings"
	"time"

	"poe2filter/internal/prices"
)

// SmartPolicy is how the shared scan servers spend a fixed search rate. A key
// whose price or listing count moves is searched more often and a quiet one
// less (never past its RefreshPolicy interval), a price jump brings the
// base's other keys forward, and a share of the searches goes to the cheap
// and empty keys seen longest ago. The app's own scanner runs without it.
type SmartPolicy struct {
	PerHour float64       // searches per hour at most; 0 leaves it to the rate limiter
	Floor   time.Duration // the shortest interval a moving key can reach
	Change  float64       // relative change in price or listings that counts as moving
	Explore float64       // share of searches for the oldest cheap or empty key
	// ExploreAge is how long a cheap or empty key must have gone unsearched
	// before an exploring search picks it.
	ExploreAge time.Duration
}

// DefaultSmart is the scan servers' policy outside a league's first week.
var DefaultSmart = SmartPolicy{PerHour: 40, Floor: 3 * time.Hour, Change: 0.3, Explore: 0.1, ExploreAge: 24 * time.Hour}

// SetSmart turns the smart policy on; nil turns it off.
func (s *Scanner) SetSmart(p *SmartPolicy) {
	s.mu.Lock()
	if p != nil {
		cp := *p
		p = &cp
	}
	s.smart = p
	s.mu.Unlock()
	s.nudge()
}

// SetPerHour changes the smart policy's search rate (a league's first week
// runs faster than the rest).
func (s *Scanner) SetPerHour(n float64) {
	s.mu.Lock()
	if s.smart != nil {
		s.smart.PerHour = n
	}
	s.mu.Unlock()
}

// SetPrior gives the previous league's prices. A new league's first pass
// searches what was valuable there first, then bases that did not exist
// there, then the rest; the item classes it learned spare the socket probes.
func (s *Scanner) SetPrior(ps []prices.ExceptionalPrice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prior = make(map[string]prices.ExceptionalPrice, len(ps))
	s.priorBases = map[string]bool{}
	for _, p := range ps {
		s.prior[keyOf(p.Base, p.Kind)+IlvlBucket{Min: p.MinIlvl, Max: p.MaxIlvl}.suffix()] = p
		s.priorBases[p.Base] = true
		if p.Class != "" && s.st.Classes[p.Base] == "" {
			s.st.Classes[p.Base] = NormalizeClass(p.Class)
		}
	}
}

// pace is the time left before the next search may start (locked).
func (s *Scanner) pace(now time.Time) time.Duration {
	if s.smart == nil || s.smart.PerHour <= 0 || s.lastSearch.IsZero() {
		return 0
	}
	return s.lastSearch.Add(s.paceSpacing()).Sub(now)
}

func (s *Scanner) paceSpacing() time.Duration {
	if s.smart == nil || s.smart.PerHour <= 0 {
		return 0
	}
	return time.Duration(float64(time.Hour) / s.smart.PerHour)
}

// smartInterval shortens a key's group interval to what its own history
// earned (locked).
func (s *Scanner) smartInterval(k *keyState, group time.Duration) time.Duration {
	if s.smart == nil || k.IntervalMin <= 0 {
		return group
	}
	iv := max(time.Duration(k.IntervalMin)*time.Minute, s.smart.Floor)
	return min(iv, group)
}

// adapt sets a fresh result's interval from the one before (locked): halved
// when the price or listing count moved, doubled when not, kept between the
// floor and the group's interval. A search that changed (quality minimum,
// query version) starts over at the group's interval.
func (s *Scanner) adapt(old, now *keyState) {
	group := s.refreshAfter(&keyState{ExceptionalPrice: now.ExceptionalPrice})
	iv := group
	if !sameSearch(old, now.ExceptionalPrice) {
		now.IntervalMin = int(iv / time.Minute)
		return
	}
	if old.IntervalMin > 0 {
		iv = min(time.Duration(old.IntervalMin)*time.Minute, group)
	}
	if s.moved(old.ExceptionalPrice, now.ExceptionalPrice) {
		iv = max(iv/2, s.smart.Floor)
	} else {
		iv = min(iv*2, group)
	}
	now.IntervalMin = int(iv / time.Minute)
}

// moved reports a change worth searching sooner for. Listing counts must move
// by three or more, and prices count only when both rest on three listings or
// more: a lone listing's price says little.
func (s *Scanner) moved(a, b prices.ExceptionalPrice) bool {
	if (a.Listings == 0) != (b.Listings == 0) {
		return true
	}
	if d := b.Listings - a.Listings; abs(d) >= 3 && float64(abs(d)) > s.smart.Change*float64(max(a.Listings, 1)) {
		return true
	}
	return a.Samples >= 3 && b.Samples >= 3 && a.ValueEx > 0 && math.Abs(b.ValueEx-a.ValueEx) > s.smart.Change*a.ValueEx
}

// jumped reports a price rise that brings the base's other keys forward:
// a build that makes one of them sought after usually lifts the rest.
func (s *Scanner) jumped(old *keyState, now prices.ExceptionalPrice) bool {
	if !sameSearch(old, now) {
		return false
	}
	a := old.ExceptionalPrice
	return a.Samples >= 3 && now.Samples >= 3 && a.ValueEx > 0 && now.ValueEx > a.ValueEx*(1+s.smart.Change)
}

// sameSearch reports whether the earlier result came from the same search: a
// failed attempt keeps the last good price, which still counts.
func sameSearch(old *keyState, now prices.ExceptionalPrice) bool {
	return old != nil && !old.ScannedAt.IsZero() && old.Min == now.Min &&
		(old.QueryVersion == scanQueryVersion || old.LastError != "")
}

// boostSiblings makes the base's other keys due at once (locked).
func (s *Scanner) boostSiblings(base, except string) int {
	n := 0
	for key, ks := range s.st.Keys {
		if key != except && ks != nil && strings.HasPrefix(key, base+"|") && !ks.Boosted {
			ks.Boosted = true
			n++
		}
	}
	return n
}

// priorRank orders a new league's first pass (locked): last league's valuable
// keys, then bases it did not have, then the rest.
func (s *Scanner) priorRank(t target) (int, float64) {
	if len(s.prior) == 0 {
		return 0, 0
	}
	if p, ok := s.prior[t.key]; ok && p.ValueEx >= s.hotEx {
		return 0, p.ValueEx
	}
	if !s.priorBases[t.base] {
		return 1, 0
	}
	return 2, 0
}

// urgency is how overdue a key is, weighted by its value (locked): a key at
// twice its interval outranks one just due, and among equally overdue keys
// the valuable go first. Nothing waits forever, since lateness keeps growing.
func (s *Scanner) urgency(t target, now time.Time) float64 {
	iv := s.refreshAfter(t.state)
	late := float64(now.Sub(t.state.LastAttempt)) / float64(max(iv, time.Minute))
	return late * (1 + math.Log10(1+t.state.ValueEx/max(s.hotEx, 1)))
}

// exploring reports whether this search goes to the oldest cheap or empty
// key (locked).
func (s *Scanner) exploring() bool {
	if s.smart == nil || s.smart.Explore <= 0 {
		return false
	}
	every := max(int(math.Round(1/s.smart.Explore)), 2)
	return s.searches%every == every-1
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
