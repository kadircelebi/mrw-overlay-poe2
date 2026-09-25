package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

const catalogBaseURL = "https://www.pathofexile.com/api/trade2/data"

type StatEntry struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Type string `json:"type"`
}

type StatGroup struct {
	ID      string      `json:"id"`
	Label   string      `json:"label"`
	Entries []StatEntry `json:"entries"`
}

type SelectOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type optionList struct {
	Options []SelectOption `json:"options"`
}

type TradeFilter struct {
	ID     string     `json:"id"`
	Text   string     `json:"text"`
	MinMax bool       `json:"minMax"`
	Option optionList `json:"option"`
	// Input is set for free-text filters (the seller account name).
	Input *FilterInput `json:"input,omitempty"`
}

type FilterInput struct {
	Placeholder string `json:"placeholder"`
}

type FilterGroup struct {
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	Filters []TradeFilter `json:"filters"`
}

type ItemEntry struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

type ItemGroup struct {
	ID      string      `json:"id"`
	Label   string      `json:"label"`
	Entries []ItemEntry `json:"entries"`
}

// Catalog is the normalized data the two overlay windows consume. The raw
// endpoint responses remain on disk so new GGG fields are not destroyed.
type Catalog struct {
	Stats   []StatGroup   `json:"stats"`
	Items   []ItemGroup   `json:"items"`
	Filters []FilterGroup `json:"filters"`
	// Currencies are the trade site's exchange items by the id listings price
	// in ("divine", "exalted"), with their name and icon.
	Currencies  []CurrencyEntry `json:"currencies"`
	UpdatedAtMs int64           `json:"updatedAtMs"`
}

type CurrencyEntry struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Image string `json:"image"`
}

type staticGroup struct {
	ID      string `json:"id"`
	Entries []struct {
		ID    string `json:"id"`
		Text  string `json:"text"`
		Image string `json:"image"`
	} `json:"entries"`
}

// iconBase serves the trade site's images (item and currency icons alike).
const iconBase = "https://web.poecdn.com"

func currenciesFrom(groups []staticGroup) []CurrencyEntry {
	out := []CurrencyEntry{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, e := range group.Entries {
			if e.ID == "" || e.Image == "" || seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			image := e.Image
			if strings.HasPrefix(image, "/") {
				image = iconBase + image
			}
			out = append(out, CurrencyEntry{ID: e.ID, Text: e.Text, Image: image})
		}
	}
	return out
}

type response[T any] struct {
	Result []T `json:"result"`
}

type CatalogStore struct {
	dir    string
	client *http.Client

	mu      sync.RWMutex
	value   Catalog
	loaded  bool
	loading chan struct{}
}

func NewCatalogStore(dataDir string) *CatalogStore {
	return &CatalogStore{
		dir:    filepath.Join(dataDir, "data"),
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

// Load returns the in-memory catalog and performs at most one concurrent disk
// or network refresh. Endpoint files are refreshed daily; stale data wins over
// a network failure so an update outage never disables price checking.
func (s *CatalogStore) Load(ctx context.Context) (Catalog, error) {
	s.mu.RLock()
	if s.loaded {
		v := s.value
		s.mu.RUnlock()
		return v, nil
	}
	loading := s.loading
	s.mu.RUnlock()
	if loading != nil {
		select {
		case <-loading:
			s.mu.RLock()
			defer s.mu.RUnlock()
			return s.value, nil
		case <-ctx.Done():
			return Catalog{}, ctx.Err()
		}
	}

	s.mu.Lock()
	if s.loaded {
		v := s.value
		s.mu.Unlock()
		return v, nil
	}
	if s.loading != nil {
		loading = s.loading
		s.mu.Unlock()
		select {
		case <-loading:
			s.mu.RLock()
			defer s.mu.RUnlock()
			return s.value, nil
		case <-ctx.Done():
			return Catalog{}, ctx.Err()
		}
	}
	s.loading = make(chan struct{})
	loading = s.loading
	s.mu.Unlock()

	value, err := s.load(ctx)
	s.mu.Lock()
	if err == nil {
		s.value = value
		s.loaded = true
	}
	close(loading)
	s.loading = nil
	s.mu.Unlock()
	return value, err
}

func (s *CatalogStore) Refresh(ctx context.Context) (Catalog, error) {
	s.mu.Lock()
	s.loaded = false
	s.mu.Unlock()
	return s.Load(ctx)
}

func (s *CatalogStore) load(ctx context.Context) (Catalog, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Catalog{}, err
	}
	statsRaw, statsAt, err := s.readOrFetch(ctx, "stats", "trade_stats.json")
	if err != nil {
		return Catalog{}, err
	}
	itemsRaw, itemsAt, err := s.readOrFetch(ctx, "items", "trade_items.json")
	if err != nil {
		return Catalog{}, err
	}
	filtersRaw, filtersAt, err := s.readOrFetch(ctx, "filters", "trade_filters.json")
	if err != nil {
		return Catalog{}, err
	}

	var stats response[StatGroup]
	var items response[ItemGroup]
	var filters response[FilterGroup]
	if err := json.Unmarshal(statsRaw, &stats); err != nil {
		return Catalog{}, fmt.Errorf("decode trade stats: %w", err)
	}
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return Catalog{}, fmt.Errorf("decode trade items: %w", err)
	}
	if err := json.Unmarshal(filtersRaw, &filters); err != nil {
		return Catalog{}, fmt.Errorf("decode trade filters: %w", err)
	}
	updated := statsAt
	if itemsAt.Before(updated) {
		updated = itemsAt
	}
	if filtersAt.Before(updated) {
		updated = filtersAt
	}
	// Currency icons are a nicety: without them prices fall back to names.
	currencies := []CurrencyEntry{}
	if staticRaw, _, err := s.readOrFetch(ctx, "static", "trade_static.json"); err == nil {
		var static response[staticGroup]
		if json.Unmarshal(staticRaw, &static) == nil {
			currencies = currenciesFrom(static.Result)
		}
	}
	return Catalog{Stats: stats.Result, Items: items.Result, Filters: filters.Result, Currencies: currencies, UpdatedAtMs: updated.UnixMilli()}, nil
}

func (s *CatalogStore) readOrFetch(ctx context.Context, endpoint, filename string) ([]byte, time.Time, error) {
	path := filepath.Join(s.dir, filename)
	var cached []byte
	var cachedAt time.Time
	if fi, err := os.Stat(path); err == nil {
		cachedAt = fi.ModTime()
		cached, _ = os.ReadFile(path)
		if len(cached) > 0 && time.Since(cachedAt) < 24*time.Hour {
			return cached, cachedAt, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogBaseURL+"/"+endpoint, nil)
	if err == nil {
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "poe2-filter/overlay")
		if resp, getErr := s.client.Do(req); getErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
				if readErr == nil && json.Valid(raw) {
					if writeErr := prices.WriteFileAtomic(path, raw); writeErr == nil {
						return raw, time.Now(), nil
					}
					return raw, time.Now(), nil
				}
			}
		}
	}
	if len(cached) > 0 {
		return cached, cachedAt, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return nil, time.Time{}, fmt.Errorf("trade catalog %s is unavailable", endpoint)
}
