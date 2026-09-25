// Package trade talks to the official PoE2 trade API and scans exceptional
// base prices within a strict share of the IP rate limit.
package trade

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rule is one "hits:period:penalty" entry from X-Rate-Limit-Ip.
type Rule struct {
	Hits    int
	Period  time.Duration
	Penalty time.Duration
}

// Limiter paces requests for one GGG rate-limit policy so that we never use
// more than Budget (0..1) of any window. The IP quota is shared with the
// user's own trade-site usage, which is visible to us only through the
// X-Rate-Limit-Ip-State counters the server returns.
type Limiter struct {
	mu           sync.Mutex
	budget       float64
	paceEvenly   bool
	rules        []Rule
	history      []time.Time // our own request times
	serverState  []int       // current hits per rule as reported by the server
	stateAt      time.Time
	blockedUntil time.Time
	// blockedBy is the window whose penalty caused blockedUntil (0 when the
	// server did not say, e.g. a bare 429 with only Retry-After).
	blockedBy time.Duration
}

// Why a request has to wait.
const (
	WaitPenalty = "penalty" // GGG restricted this IP
	WaitShared  = "shared"  // the server's counter (all apps on this IP) is at our budget
	WaitOurs    = "ours"    // our own requests are at our budget
	WaitPacing  = "pacing"  // background scanner spacing
)

type waitCause struct {
	kind   string
	window time.Duration
}

// NewLimiter creates a limiter seeded with known rules; the server's headers
// replace them after the first response.
func NewLimiter(budget float64, seed []Rule) *Limiter {
	if budget <= 0 || budget > 1 {
		budget = 0.5
	}
	return &Limiter{budget: budget, paceEvenly: true, rules: seed}
}

// SetEvenPacing controls whether requests are spread uniformly across every
// rate-limit window. Background scanners should stay evenly paced; interactive
// searches may use GGG's permitted short bursts and are still stopped by the
// per-window counters below.
func (l *Limiter) SetEvenPacing(enabled bool) {
	l.mu.Lock()
	l.paceEvenly = enabled
	l.mu.Unlock()
}

// SetBudget changes the share of the quota we may use.
func (l *Limiter) SetBudget(b float64) {
	if b <= 0 || b > 1 {
		return
	}
	l.mu.Lock()
	l.budget = b
	l.mu.Unlock()
}

func (l *Limiter) allowed(r Rule) int {
	return max(1, int(math.Floor(float64(r.Hits)*l.budget)))
}

// nextSlot returns the earliest time a request may be sent (locked).
func (l *Limiter) nextSlot(now time.Time) time.Time {
	next, _ := l.nextSlotWhy(now)
	return next
}

// nextSlotWhy also reports which limit decided the slot.
func (l *Limiter) nextSlotWhy(now time.Time) (time.Time, waitCause) {
	next := now
	var why waitCause
	if l.blockedUntil.After(next) {
		next = l.blockedUntil
		why = waitCause{WaitPenalty, l.blockedBy}
	}
	for i, r := range l.rules {
		allowed := l.allowed(r)

		// Spread requests evenly instead of bursting to the limit.
		if l.paceEvenly {
			if n := len(l.history); n > 0 {
				spaced := l.history[n-1].Add(r.Period / time.Duration(allowed))
				if spaced.After(next) {
					next, why = spaced, waitCause{WaitPacing, r.Period}
				}
			}
		}

		// Our own requests inside the window.
		var ours []time.Time
		for _, t := range l.history {
			if now.Sub(t) < r.Period {
				ours = append(ours, t)
			}
		}
		if len(ours) >= allowed {
			if t := ours[len(ours)-allowed].Add(r.Period); t.After(next) {
				next, why = t, waitCause{WaitOurs, r.Period}
			}
		}

		// Server-reported usage (includes the user's own searches). Be
		// conservative: assume those hits stay until their window passes.
		if i < len(l.serverState) && now.Sub(l.stateAt) < r.Period && l.serverState[i] >= allowed {
			if t := l.stateAt.Add(r.Period); t.After(next) {
				next, why = t, waitCause{WaitShared, r.Period}
			}
		}
	}
	return next, why
}

// Wait blocks until a request may be sent, then records it.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := time.Now()
		slot := l.nextSlot(now)
		if !slot.After(now) {
			l.history = append(l.history, now)
			l.trimHistory(now)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()

		timer := time.NewTimer(time.Until(slot))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Spacing is the steady-state gap between requests within the budget.
func (l *Limiter) Spacing() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	var d time.Duration
	for _, r := range l.rules {
		d = max(d, r.Period/time.Duration(l.allowed(r)))
	}
	return d
}

// NextIn reports how long until the next request could be sent.
func (l *Limiter) NextIn() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return max(0, time.Until(l.nextSlot(time.Now())))
}

func (l *Limiter) trimHistory(now time.Time) {
	var longest time.Duration
	for _, r := range l.rules {
		longest = max(longest, r.Period)
	}
	keep := l.history[:0]
	for _, t := range l.history {
		if now.Sub(t) < longest {
			keep = append(keep, t)
		}
	}
	l.history = keep
}

// Observe updates the limiter from a response's rate-limit headers.
func (l *Limiter) Observe(resp *http.Response) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	if rules := parseRules(resp.Header.Get("X-Rate-Limit-Ip")); len(rules) > 0 {
		l.rules = rules
	}
	namedBy := false
	if state := resp.Header.Get("X-Rate-Limit-Ip-State"); state != "" {
		var hits []int
		for _, part := range strings.Split(state, ",") {
			f := strings.Split(strings.TrimSpace(part), ":")
			if len(f) != 3 {
				continue
			}
			n, _ := strconv.Atoi(f[0])
			hits = append(hits, n)
			// A non-zero third field means we are currently restricted.
			if secs, _ := strconv.Atoi(f[2]); secs > 0 {
				if t := now.Add(time.Duration(secs) * time.Second); t.After(l.blockedUntil) {
					l.blockedUntil = t
					period, _ := strconv.Atoi(f[1])
					l.blockedBy = time.Duration(period) * time.Second
					namedBy = true
				}
			}
		}
		l.serverState = hits
		l.stateAt = now
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := 60 * time.Second
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
		if t := now.Add(wait); t.After(l.blockedUntil) {
			l.blockedUntil = t
			if !namedBy {
				l.blockedBy = 0
			}
		}
	}
}

// QuotaWindow is one GGG rate-limit window as last seen.
type QuotaWindow struct {
	PeriodSec  int `json:"periodSec"`
	Limit      int `json:"limit"`
	Allowed    int `json:"allowed"` // our budgeted share of Limit
	Hits       int `json:"hits"`    // estimated current use by everything on this IP
	PenaltySec int `json:"penaltySec"`
}

// QuotaStatus explains the quota for diagnostics: how full each window is,
// whether GGG restricted the IP, and what the next request would wait for.
type QuotaStatus struct {
	Windows    []QuotaWindow `json:"windows"`
	ObservedAt time.Time     `json:"observedAt"` // zero before the first response
	// RestrictedUntil is set while GGG penalises the IP; RestrictedWindowSec
	// names the window that was exceeded (0 when GGG did not say).
	RestrictedUntil     time.Time `json:"restrictedUntil"`
	RestrictedWindowSec int       `json:"restrictedWindowSec"`
	WaitSec             int       `json:"waitSec"`
	WaitReason          string    `json:"waitReason"`
	WaitWindowSec       int       `json:"waitWindowSec"`
}

// Status estimates the current use of every window: the server's last count
// while its window has not passed, plus our own requests sent since then.
func (l *Limiter) Status() QuotaStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	st := QuotaStatus{ObservedAt: l.stateAt}
	for i, r := range l.rules {
		w := QuotaWindow{PeriodSec: int(r.Period / time.Second), Limit: r.Hits, Allowed: l.allowed(r), PenaltySec: int(r.Penalty / time.Second)}
		since := time.Time{}
		if i < len(l.serverState) && now.Sub(l.stateAt) < r.Period {
			w.Hits, since = l.serverState[i], l.stateAt
		}
		for _, t := range l.history {
			if t.After(since) && now.Sub(t) < r.Period {
				w.Hits++
			}
		}
		st.Windows = append(st.Windows, w)
	}
	if l.blockedUntil.After(now) {
		st.RestrictedUntil = l.blockedUntil
		st.RestrictedWindowSec = int(l.blockedBy / time.Second)
	}
	if slot, why := l.nextSlotWhy(now); slot.After(now) {
		st.WaitSec = int((slot.Sub(now) + time.Second - 1) / time.Second)
		st.WaitReason = why.kind
		st.WaitWindowSec = int(why.window / time.Second)
	}
	return st
}

func parseRules(h string) []Rule {
	var rules []Rule
	for _, part := range strings.Split(h, ",") {
		f := strings.Split(strings.TrimSpace(part), ":")
		if len(f) != 3 {
			continue
		}
		hits, e1 := strconv.Atoi(f[0])
		period, e2 := strconv.Atoi(f[1])
		penalty, e3 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || e3 != nil || hits <= 0 || period <= 0 {
			continue
		}
		rules = append(rules, Rule{
			Hits:    hits,
			Period:  time.Duration(period) * time.Second,
			Penalty: time.Duration(penalty) * time.Second,
		})
	}
	return rules
}

// Seed rules observed on 2026-09-18; replaced by live headers after the
// first response.
var (
	SearchSeedRules = parseRules("5:10:60,15:60:300,30:300:1800,600:21600:3600")
	FetchSeedRules  = parseRules("12:4:10,16:12:300,50:300:300,1000:21600:1800")
)
