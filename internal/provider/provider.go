// Package provider decides where the price snapshot comes from. The filter
// generator only ever sees a *prices.Snapshot; switching from local collection
// to a collector server means putting a Remote provider first in the chain.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	switch {
	case l.Exceptional != nil:
		snap.Exceptional = l.Exceptional.Results()
		snap.Sources[collector.SourceExceptional] = prices.SourceStatus{OK: true, FetchedAt: snap.GeneratedAt}
	case prev != nil && prev.League == snap.League:
		snap.Exceptional = prev.Exceptional
	}
	if err := prices.Save(l.CachePath, snap); err != nil {
		return nil, fmt.Errorf("could not write the cache: %w", err)
	}
	return snap, nil
}

// Remote downloads a snapshot published by a collector server.
type Remote struct {
	URL    string
	League string
	// CachePath, when set, stores the downloaded snapshot for offline use.
	CachePath string
}

func (r Remote) Name() string { return "sunucu" }

func (r Remote) Get(ctx context.Context) (*prices.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", collector.UserAgent)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var s prices.Snapshot
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if r.League != "" && s.League != r.League {
		return nil, fmt.Errorf("the server publishes a different league (%s)", s.League)
	}
	if time.Since(s.GeneratedAt) > 24*time.Hour {
		return nil, fmt.Errorf("the data on the server is older than 24 hours")
	}
	if r.CachePath != "" {
		_ = prices.Save(r.CachePath, &s)
	}
	return &s, nil
}
