package collector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

// Source names used in Snapshot.Sources.
const (
	SourceNinja         = "poe.ninja"
	SourceScout         = "poe2scout"
	SourceNinjaExchange = "poe.ninja/exchange"
	SourceExceptional   = "trade-exceptional"
)

// Options configures a collection run.
type Options struct {
	League string
	HTTP   *http.Client
	Log    func(string)
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

// Collect builds a fresh market snapshot. When a source fails, the matching
// section of prev (the last good snapshot, may be nil) is carried over and the
// failure is recorded in Sources, so one flaky upstream never empties a section.
// Exceptional prices are not collected here; the caller attaches them.
func Collect(ctx context.Context, opt Options, prev *prices.Snapshot) (*prices.Snapshot, error) {
	if opt.HTTP == nil {
		opt.HTTP = NewHTTPClient()
	}
	now := time.Now().UTC()

	var (
		wg       sync.WaitGroup
		ninja    NinjaResult
		scout    []prices.CurrencyPrice
		scoutDiv float64
		scoutErr error
		exchange []NinjaExchangePrice
		exDiv    float64
		exFailed map[string]error
	)
	wg.Add(3)
	go func() { defer wg.Done(); ninja = FetchNinjaUniques(ctx, opt.HTTP, opt.League) }()
	go func() { defer wg.Done(); scout, scoutDiv, scoutErr = FetchScout(ctx, opt.HTTP, opt.League) }()
	go func() {
		defer wg.Done()
		exchange, exDiv, exFailed = FetchNinjaExchange(ctx, opt.HTTP, opt.League, NinjaExchangeTypes)
	}()
	wg.Wait()

	snap := &prices.Snapshot{
		SchemaVersion: prices.SchemaVersion,
		League:        opt.League,
		GeneratedAt:   now,
		Sources:       map[string]prices.SourceStatus{},
	}
	usablePrev := prev != nil && prev.League == opt.League

	// Rates: poe.ninja first, poe2scout second, previous snapshot last.
	switch {
	case ninja.DivineEx > 0:
		snap.Rates = prices.Rates{DivineEx: ninja.DivineEx, ChaosEx: ninja.ChaosEx}
	case scoutDiv > 0:
		snap.Rates = prices.Rates{DivineEx: scoutDiv}
	case exDiv > 0:
		snap.Rates = prices.Rates{DivineEx: exDiv}
	case usablePrev:
		snap.Rates = prev.Rates
	default:
		return nil, errors.New("no source provided the divine rate")
	}
	if snap.Rates.ChaosEx <= 0 {
		for _, c := range scout {
			if c.APIID == "chaos" {
				snap.Rates.ChaosEx = c.ValueEx
			}
		}
	}

	// Currency and other bulk items (poe2scout).
	if scoutErr == nil {
		snap.Currency = scout
		snap.Sources[SourceScout] = prices.SourceStatus{OK: true, FetchedAt: now}
		opt.logf("      poe2scout: %d toplu eşya alındı.", len(scout))
	} else {
		st := prices.SourceStatus{Error: scoutErr.Error()}
		if usablePrev {
			snap.Currency = prev.Currency
			st.FetchedAt = prev.Sources[SourceScout].FetchedAt
		}
		snap.Sources[SourceScout] = st
		opt.logf("      [Uyarı] poe2scout alınamadı: %v (önceki veri kullanılıyor: %t)", scoutErr, usablePrev)
	}

	// poe.ninja's exchange fills the holes poe2scout leaves. It only adds names
	// that are missing, so the prices the thresholds were tuned against stay put.
	if len(exchange) > 0 {
		before := len(snap.Currency)
		snap.Currency = MergeExchangePrices(snap.Currency, exchange, snap.Rates.DivineEx)
		if added := len(snap.Currency) - before; added > 0 {
			opt.logf("      poe.ninja exchange: %d eşya eklendi (poe2scout'ta yoktu).", added)
		}
		snap.Sources[SourceNinjaExchange] = prices.SourceStatus{OK: true, FetchedAt: now}
	}
	for t, err := range exFailed {
		opt.logf("      [Uyarı] poe.ninja %s alınamadı: %v", t, err)
	}

	// Uniques (poe.ninja). Failed categories fall back to the previous snapshot
	// per category, so a single failed request cannot hide a whole category.
	uniques := ninja.ToExalted(snap.Rates.DivineEx)
	if len(ninja.Failed) > 0 && usablePrev {
		for base, ub := range prev.UniqueBases {
			for _, u := range ub.Uniques {
				if _, failed := ninja.Failed[u.Category]; failed {
					uniques[base] = append(uniques[base], u)
				}
			}
		}
	}
	snap.UniqueBases = AggregateUniqueBases(uniques)
	if len(ninja.Failed) == 0 {
		snap.Sources[SourceNinja] = prices.SourceStatus{OK: true, FetchedAt: now}
	} else {
		failed := make([]string, 0, len(ninja.Failed))
		for typ, err := range ninja.Failed {
			failed = append(failed, fmt.Sprintf("%s (%v)", typ, err))
		}
		sort.Strings(failed)
		snap.Sources[SourceNinja] = prices.SourceStatus{
			OK:        len(ninja.Failed) < len(NinjaUniqueTypes),
			FetchedAt: now,
			Error:     "başarısız: " + strings.Join(failed, ", "),
		}
		opt.logf("      [Uyarı] poe.ninja kategorileri alınamadı: %s", strings.Join(failed, ", "))
	}
	opt.logf("      poe.ninja: %d farklı taban (1 Divine = %.1f Exalt).", len(snap.UniqueBases), snap.Rates.DivineEx)

	if err := snap.Validate(); err != nil {
		return nil, err
	}
	return snap, nil
}
