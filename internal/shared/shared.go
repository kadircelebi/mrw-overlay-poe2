// Package shared downloads the exceptional base prices the scan servers
// publish (cmd/scanner) and merges them. Each server uploads its share of the
// keys as exceptional-<name>.json.gz to one GitHub release; the app lists that
// release, downloads what changed and keeps the newest scan of every key.
// With these prices a player's own PC does not need to spend its trade quota.
package shared

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
	"poe2filter/internal/useragent"
)

const (
	DefaultRepo = "kadircelebi/poe2-filtre-data"
	DefaultTag  = "data"
	// checkEvery is how often the release is listed; the servers upload
	// every six hours.
	checkEvery = time.Hour
	maxFile    = 32 << 20
)

var fileRE = regexp.MustCompile(`^exceptional-[a-z0-9_-]{1,32}\.json\.gz$`)

// pricesFile is the hourly currency/unique snapshot one server publishes.
const pricesFile = "prices.json.gz"

func wanted(name string) bool { return fileRE.MatchString(name) || name == pricesFile }

// Status is what the panel shows about the shared prices.
type Status struct {
	Files     int    `json:"files"`
	Keys      int    `json:"keys"`
	NewestMs  int64  `json:"newestMs"`  // newest scan among the keys
	CheckedMs int64  `json:"checkedMs"` // last successful look at the release
	League    string `json:"league"`
	Error     string `json:"error,omitempty"`
}

// Store keeps the downloaded files in Dir and their merged prices in memory.
type Store struct {
	Dir  string
	Repo string
	Tag  string
	API  string // https://api.github.com (tests override it)
	HTTP *http.Client

	mu      sync.Mutex
	league  string
	checked time.Time
	results []prices.ExceptionalPrice
	status  Status
}

func New(dir string) *Store {
	return &Store{Dir: dir, Repo: DefaultRepo, Tag: DefaultTag, API: "https://api.github.com",
		HTTP: &http.Client{Timeout: 60 * time.Second}}
}

type releaseAsset struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"browser_download_url"`
}

// Refresh lists the release (at most hourly per league), downloads the files
// that changed and merges them. A network failure keeps what was downloaded
// before: stale prices beat none.
func (s *Store) Refresh(ctx context.Context, league string) error {
	s.mu.Lock()
	fresh := s.league == league && time.Since(s.checked) < checkEvery
	s.mu.Unlock()
	if fresh {
		return nil
	}
	err := s.download(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.league = league
	s.load()
	if err != nil {
		s.status.Error = err.Error()
		return err
	}
	s.checked = time.Now()
	s.status.CheckedMs = s.checked.UnixMilli()
	s.status.Error = ""
	return nil
}

func (s *Store) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", useragent.Value())
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", strings.SplitN(url, "?", 2)[0], resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFile))
}

func (s *Store) download(ctx context.Context) error {
	raw, err := s.get(ctx, fmt.Sprintf("%s/repos/%s/releases/tags/%s", strings.TrimRight(s.API, "/"), s.Repo, s.Tag))
	if err != nil {
		return err
	}
	var rel struct {
		Assets []releaseAsset `json:"assets"`
	}
	if err := json.Unmarshal(raw, &rel); err != nil {
		return fmt.Errorf("release list: %w", err)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, a := range rel.Assets {
		if !wanted(a.Name) || a.Size > maxFile || !strings.HasPrefix(a.URL, "https://") {
			continue
		}
		keep[a.Name] = true
		path := filepath.Join(s.Dir, a.Name)
		stamp := a.UpdatedAt.UTC().Format(time.RFC3339)
		if old, err := os.ReadFile(path + ".stamp"); err == nil && string(old) == stamp {
			if _, err := os.Stat(path); err == nil {
				continue
			}
		}
		data, err := s.get(ctx, a.URL)
		if err != nil {
			return err
		}
		if a.Name == pricesFile {
			if _, err := decodePrices(data); err != nil {
				return fmt.Errorf("%s: %w", a.Name, err)
			}
		} else if _, err := decode(data); err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		if err := prices.WriteFileAtomic(path, data); err != nil {
			return err
		}
		_ = os.WriteFile(path+".stamp", []byte(stamp), 0o644)
	}
	// A server that stopped publishing leaves nothing stale behind.
	entries, _ := os.ReadDir(s.Dir)
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".stamp")
		if wanted(name) && !keep[name] {
			_ = os.Remove(filepath.Join(s.Dir, e.Name()))
		}
	}
	return nil
}

func gunzip(gz []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(zr, 8*maxFile))
}

func decodePrices(gz []byte) (*prices.Snapshot, error) {
	raw, err := gunzip(gz)
	if err != nil {
		return nil, err
	}
	var snap prices.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	if err := snap.Validate(); err != nil {
		return nil, err
	}
	return &snap, nil
}

// Prices returns the published currency/unique snapshot when it is for the
// league and not older than maxAge; otherwise the caller collects itself.
func (s *Store) Prices(league string, maxAge time.Duration) (*prices.Snapshot, error) {
	raw, err := os.ReadFile(filepath.Join(s.Dir, pricesFile))
	if err != nil {
		return nil, fmt.Errorf("no shared prices yet")
	}
	snap, err := decodePrices(raw)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(snap.League), strings.TrimSpace(league)) {
		return nil, fmt.Errorf("shared prices are for %s", snap.League)
	}
	if age := time.Since(snap.GeneratedAt); age > maxAge {
		return nil, fmt.Errorf("shared prices are %s old", age.Round(time.Minute))
	}
	return snap, nil
}

// shareFile is the scanner's export format (trade.Share).
type shareFile struct {
	Version int    `json:"version"`
	League  string `json:"league"`
	Keys    map[string]*struct {
		prices.ExceptionalPrice
	} `json:"keys"`
}

func decode(gz []byte) (*shareFile, error) {
	raw, err := gunzip(gz)
	if err != nil {
		return nil, err
	}
	var f shareFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.Version != trade.ShareVersion {
		return nil, fmt.Errorf("format %d, want %d", f.Version, trade.ShareVersion)
	}
	return &f, nil
}

// load merges the downloaded files for the current league (locked): the
// newest scan of a key wins, as when importing a shared scan.
func (s *Store) load() {
	merged := map[string]prices.ExceptionalPrice{}
	files := 0
	entries, _ := os.ReadDir(s.Dir)
	for _, e := range entries {
		if !fileRE.MatchString(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}
		f, err := decode(raw)
		if err != nil || !strings.EqualFold(strings.TrimSpace(f.League), strings.TrimSpace(s.league)) {
			continue
		}
		files++
		for key, v := range f.Keys {
			if v == nil || v.ScannedAt.IsZero() {
				continue
			}
			if cur, ok := merged[key]; !ok || v.ScannedAt.After(cur.ScannedAt) {
				merged[key] = v.ExceptionalPrice
			}
		}
	}
	out := make([]prices.ExceptionalPrice, 0, len(merged))
	var newest time.Time
	for _, p := range merged {
		out = append(out, p)
		if p.ScannedAt.After(newest) {
			newest = p.ScannedAt
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Base != b.Base {
			return a.Base < b.Base
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.MinIlvl < b.MinIlvl
	})
	s.results = out
	s.status.Files, s.status.Keys, s.status.League = files, len(out), s.league
	s.status.NewestMs = 0
	if !newest.IsZero() {
		s.status.NewestMs = newest.UnixMilli()
	}
}

// LoadCached merges what earlier runs downloaded, without the network, so
// the app knows at start whether its own scanner is needed.
func (s *Store) LoadCached(league string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.league == league && len(s.results) > 0 {
		return
	}
	s.league = league
	s.load()
}

// Results returns the merged prices of the league last refreshed.
func (s *Store) Results() []prices.ExceptionalPrice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]prices.ExceptionalPrice(nil), s.results...)
}

// Covers reports whether shared prices exist for the league, which lets the
// player's own scanner stay idle.
func (s *Store) Covers(league string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.EqualFold(s.league, league) && len(s.results) > 0
}

func (s *Store) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}
