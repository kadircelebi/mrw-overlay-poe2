// Package provider decides where the price snapshot comes from. The filter
// generator only ever sees a *prices.Snapshot.
package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"poe2filter/internal/collector"
	"poe2filter/internal/prices"

	"poe2filter/internal/i18n"
)

// Provider returns a price snapshot.
type Provider interface {
	Name() string
	Get(ctx context.Context) (*prices.Snapshot, error)
}

// Chain tries providers in order and returns the first valid snapshot.
type Chain []Provider

// Get returns the first successful snapshot and the name of its provider.
func (c Chain) Get(ctx context.Context) (*prices.Snapshot, string, error) {
	var errs []string
	for _, p := range c {
		s, err := p.Get(ctx)
		if err == nil {
			return s, p.Name(), nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.Name(), err))
	}
	return nil, "", errors.New(i18n.T("err.noPriceSource") + strings.Join(errs, "; "))
}

// Cache serves the last snapshot saved on disk.
type Cache struct{ Path string }

func (c Cache) Name() string { return "önbellek" }

func (c Cache) Get(ctx context.Context) (*prices.Snapshot, error) { return prices.Load(c.Path) }

// ExceptionalSource supplies exceptional base prices (the local trade scanner).
type ExceptionalSource interface {
	Results() []prices.ExceptionalPrice
}

// Local collects prices directly from upstream sources on this machine and
// saves the result to the cache.
type Local struct {
	Options     collector.Options
	CachePath   string
	Exceptional ExceptionalSource // may be nil
}

func (l Local) Name() string { return "yerel toplama" }

func (l Local) Get(ctx context.Context) (*prices.Snapshot, error) {
	prev, _ := prices.Load(l.CachePath)
	snap, err := collector.Collect(ctx, l.Options, prev)
	if err != nil {
		return nil, err
	}
	return attachAndSave(snap, prev, l.Exceptional, l.CachePath)
}

// attachAndSave adds the exceptional prices to a market snapshot (or keeps
// the previous ones for the same league) and writes the cache.
func attachAndSave(snap, prev *prices.Snapshot, ex ExceptionalSource, cachePath string) (*prices.Snapshot, error) {
	if snap.Sources == nil {
		snap.Sources = map[string]prices.SourceStatus{}
	}
	switch {
	case ex != nil:
		snap.Exceptional = ex.Results()
		snap.Sources[collector.SourceExceptional] = prices.SourceStatus{OK: true, FetchedAt: snap.GeneratedAt}
	case prev != nil && prev.League == snap.League:
		snap.Exceptional = prev.Exceptional
	}
	if err := prices.Save(cachePath, snap); err != nil {
		return nil, fmt.Errorf("could not write the cache: %w", err)
	}
	return snap, nil
}

// SharedPrices is the hourly snapshot the scan servers publish; when it is
// missing or stale the chain falls through to Local.
type SharedPrices struct {
	Store interface {
		Prices(league string, maxAge time.Duration) (*prices.Snapshot, error)
	}
	League      string
	MaxAge      time.Duration
	CachePath   string
	Exceptional ExceptionalSource // may be nil
}

func (p SharedPrices) Name() string { return "paylaşılan sunucular" }

func (p SharedPrices) Get(ctx context.Context) (*prices.Snapshot, error) {
	snap, err := p.Store.Prices(p.League, p.MaxAge)
	if err != nil {
		return nil, err
	}
	prev, _ := prices.Load(p.CachePath)
	return attachAndSave(snap, prev, p.Exceptional, p.CachePath)
}
