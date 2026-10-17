package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"poe2filter/internal/prices"
	"poe2filter/internal/useragent"
)

// GGG publishes the Currency Exchange as hourly digests of completed trades,
// one file per hour for every league (about 2.5 MB). A digest never changes
// once published, so each hour is fetched once and kept on disk; the current
// hour is not published until it ends.
const (
	cxURL        = "https://web.poecdn.com/api/currency-exchange/poe2/"
	baseItemsURL = "https://repoe-fork.github.io/poe2/base_items.min.json"
	// cxKeep is how long fetched hours stay on disk: the day the weak items
	// fall back to, and the hour still being published.
	cxKeep = (cxDayHours + 2) * time.Hour
	// baseItemsEvery is how often the trade id to name list is refreshed.
	baseItemsEvery = 24 * time.Hour
)

// CXSource reads the Currency Exchange for the collector. It keeps fetched
// hours and the item name list under Dir.
type CXSource struct {
	Dir  string
	HTTP *http.Client
	Now  func() time.Time
	// Hours is how many published hours Prices reads; 0 means cxDayHours.
	// The app reads cxWindowHours: the weak items' day fallback is the
	// price server's.
	Hours int
}

// NewCXSource keeps its files under dir.
func NewCXSource(dir string) *CXSource {
	return &CXSource{Dir: dir, HTTP: &http.Client{Timeout: 90 * time.Second}, Now: time.Now}
}

// CXResult is the exchange's view of one league: the bases' rates and the
// price of every item that traded, by item name, with its item class.
type CXResult struct {
	Rates   prices.Rates
	Items   map[string]prices.CurrencyPrice
	Classes map[string]string
	// Weak names the items no market saw cxMinUnits of: another source's
	// price is kept for them when there is one.
	Weak  map[string]bool
	Hours int
}

// NewAppCXSource is the app's own reading of the exchange, for when no
// recent shared prices are at hand: the price window only.
func NewAppCXSource(dir string) *CXSource {
	s := NewCXSource(dir)
	s.Hours = cxWindowHours
	return s
}

// Prices reads the last cxWindowHours published hours of league, and the
// last cxDayHours for the items those hours saw too few trades of.
func (s *CXSource) Prices(ctx context.Context, league string) (CXResult, error) {
	names, err := s.baseItems(ctx)
	if err != nil {
		return CXResult{}, fmt.Errorf("item names: %w", err)
	}
	want := s.Hours
	if want <= 0 {
		want = cxDayHours
	}
	now := s.Now().UTC().Truncate(time.Hour)
	var hours [][]cxTrade
	got := 0
	for back := 1; back <= want+1 && got < want; back++ {
		at := now.Add(-time.Duration(back) * time.Hour).Unix()
		markets, err := s.hour(ctx, at)
		if err != nil {
			// The newest hour may not be out yet; an older gap is skipped.
			continue
		}
		var trades []cxTrade
		for _, m := range markets {
			if m.League != league || len(m.Pair) != 2 {
				continue
			}
			a, b := m.Pair[0], m.Pair[1]
			trades = append(trades, cxTrade{A: a, B: b, QA: m.Volume[a], QB: m.Volume[b]})
		}
		hours = append(hours, trades)
		got++
	}
	if got == 0 {
		return CXResult{}, errors.New("no published hour could be read")
	}
	s.prune()
	vol := cxVolumes(hours)
	res := CXResult{Rates: cxRates(vol), Items: map[string]prices.CurrencyPrice{}, Classes: map[string]string{}, Weak: map[string]bool{}, Hours: min(got, cxWindowHours)}
	if res.Rates.DivineEx <= 0 {
		return CXResult{}, errors.New("no Divine Orb trades in the window")
	}
	items, weak := cxItemPricesWeak(vol, res.Rates)
	cxDayFallback(items, weak, hours)
	for id, p := range items {
		item, ok := names[id]
		if !ok || item.Name == "" {
			continue
		}
		p.Name = item.Name
		res.Items[item.Name] = p
		res.Classes[item.Name] = item.Class
		if weak[id] {
			res.Weak[item.Name] = true
		}
	}
	return res, nil
}

type cxMarket struct {
	League string             `json:"league"`
	Pair   []string           `json:"market_pair"`
	Volume map[string]float64 `json:"volume_traded"`
}

// hour returns the markets of the digest starting at unix time at, from
// disk when it was fetched before.
func (s *CXSource) hour(ctx context.Context, at int64) ([]cxMarket, error) {
	path := filepath.Join(s.Dir, "hours", strconv.FormatInt(at, 10)+".json.gz")
	if raw, err := readGzip(path); err == nil {
		return decodeCXHour(raw, at)
	}
	raw, err := s.get(ctx, cxURL+strconv.FormatInt(at, 10))
	if err != nil {
		return nil, err
	}
	markets, err := decodeCXHour(raw, at)
	if err != nil {
		return nil, err
	}
	_ = writeGzip(path, raw)
	return markets, nil
}

// decodeCXHour reads a digest. GGG answers an hour it has not published yet
// with the latest one's id and no markets; that is not kept.
func decodeCXHour(raw []byte, at int64) ([]cxMarket, error) {
	var d struct {
		Next    json.Number `json:"next_change_id"`
		Markets []cxMarket  `json:"markets"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	if len(d.Markets) == 0 {
		return nil, fmt.Errorf("hour %d not published yet", at)
	}
	return d.Markets, nil
}

type baseItem struct {
	Name  string `json:"name"`
	Class string `json:"item_class"`
}

// baseItems maps trade ids ("Metadata/Items/Currency/…") to item names,
// refreshed daily; a stale list is used when the refresh fails.
func (s *CXSource) baseItems(ctx context.Context) (map[string]baseItem, error) {
	path := filepath.Join(s.Dir, "base_items.json.gz")
	info, statErr := os.Stat(path)
	if statErr != nil || s.Now().Sub(info.ModTime()) > baseItemsEvery {
		if raw, err := s.get(ctx, baseItemsURL); err == nil {
			var all map[string]baseItem
			if json.Unmarshal(raw, &all) == nil && len(all) > 0 {
				small, _ := json.Marshal(all)
				_ = writeGzip(path, small)
				return all, nil
			}
		}
	}
	raw, err := readGzip(path)
	if err != nil {
		return nil, err
	}
	var all map[string]baseItem
	return all, json.Unmarshal(raw, &all)
}

func (s *CXSource) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", useragent.Value())
	req.Header.Set("Accept", "application/json")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	return body, nil
}

// prune drops fetched hours older than cxKeep.
func (s *CXSource) prune() {
	dir := filepath.Join(s.Dir, "hours")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := s.Now().Add(-cxKeep).Unix()
	for _, e := range entries {
		at, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".json.gz"), 10, 64)
		if err == nil && at < cutoff {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func readGzip(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

func writeGzip(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return prices.WriteFileAtomic(path, buf.Bytes())
}
