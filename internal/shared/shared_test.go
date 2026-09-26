package shared

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

type fakeRelease struct {
	mu       sync.Mutex
	files    map[string][]byte
	updated  map[string]time.Time
	listings int
	gets     map[string]int
	down     bool
}

func gz(t *testing.T, league string, keys map[string]map[string]any) []byte {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"version": 1, "league": league, "keys": keys})
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()
	return buf.Bytes()
}

func (f *fakeRelease) server(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.down {
			http.Error(w, "down", 503)
			return
		}
		if r.URL.Path == "/repos/me/data/releases/tags/data" {
			f.listings++
			var assets []map[string]any
			for name, data := range f.files {
				assets = append(assets, map[string]any{"name": name, "size": len(data),
					"updated_at": f.updated[name].Format(time.RFC3339), "browser_download_url": srv.URL + "/dl/" + name})
			}
			json.NewEncoder(w).Encode(map[string]any{"assets": assets})
			return
		}
		name := r.URL.Path[len("/dl/"):]
		f.gets[name]++
		w.Write(f.files[name])
	}))
	return srv
}

func newStore(t *testing.T, srv *httptest.Server) *Store {
	s := New(t.TempDir())
	s.Repo, s.API = "me/data", srv.URL
	s.HTTP = srv.Client() // trusts the test server's certificate
	return s
}

func key(base string, ex float64, at time.Time, minIlvl int) map[string]any {
	return map[string]any{"base": base, "kind": "quality", "min": 21, "min_ilvl": minIlvl, "value_ex": ex,
		"listings": 20, "samples": 8, "scanned_at": at.Format(time.RFC3339)}
}

func TestSharedFilesAreMergedNewestFirst(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := &fakeRelease{gets: map[string]int{}, updated: map[string]time.Time{}, files: map[string][]byte{
		"exceptional-office.json.gz": gz(t, "Forbidden Rites", map[string]map[string]any{
			"Vaal Cuirass|quality|ilvl82+": key("Vaal Cuirass", 100, now.Add(-time.Hour), 82),
			"Corsair Coat|quality|ilvl82+": key("Corsair Coat", 64, now, 82),
		}),
		"exceptional-home.json.gz": gz(t, "Forbidden Rites", map[string]map[string]any{
			"Vaal Cuirass|quality|ilvl82+": key("Vaal Cuirass", 120, now, 82), // newer: wins
		}),
		"exceptional-old.json.gz": gz(t, "Old League", map[string]map[string]any{
			"Old|quality": key("Old", 1, now, 0),
		}),
		"notes.txt": []byte("ignored"),
	}}
	for name := range f.files {
		f.updated[name] = now
	}
	srv := f.server(t)
	defer srv.Close()
	s := newStore(t, srv)
	s.Dir = t.TempDir()

	if err := s.Refresh(context.Background(), "Forbidden Rites"); err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, p := range s.Results() {
		got[p.Base] = p.ValueEx
	}
	if len(got) != 2 || got["Vaal Cuirass"] != 120 || got["Corsair Coat"] != 64 {
		t.Fatalf("merged = %v", got)
	}
	st := s.Status()
	if st.Files != 2 || st.Keys != 2 || st.NewestMs != now.UnixMilli() || !s.Covers("forbidden rites") {
		t.Fatalf("status = %+v", st)
	}
	if s.Covers("Standard") {
		t.Fatal("another league is covered")
	}
}

func TestSharedRefreshDownloadsOnlyWhatChanged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := &fakeRelease{gets: map[string]int{}, updated: map[string]time.Time{}, files: map[string][]byte{
		"exceptional-office.json.gz": gz(t, "L", map[string]map[string]any{"A|quality": key("A", 5, now, 0)}),
		"exceptional-home.json.gz":   gz(t, "L", map[string]map[string]any{"B|quality": key("B", 7, now, 0)}),
	}}
	for name := range f.files {
		f.updated[name] = now
	}
	srv := f.server(t)
	defer srv.Close()
	s := newStore(t, srv)

	if err := s.Refresh(context.Background(), "L"); err != nil {
		t.Fatal(err)
	}
	// Within the hour nothing is asked again.
	s.Refresh(context.Background(), "L")
	if f.listings != 1 {
		t.Fatalf("release listed %d times", f.listings)
	}
	// An hour later only the file that changed is downloaded; a file that
	// left the release is dropped.
	s.checked = s.checked.Add(-2 * time.Hour)
	f.mu.Lock()
	f.files["exceptional-office.json.gz"] = gz(t, "L", map[string]map[string]any{"A|quality": key("A", 9, now.Add(time.Minute), 0)})
	f.updated["exceptional-office.json.gz"] = now.Add(time.Minute)
	delete(f.files, "exceptional-home.json.gz")
	f.mu.Unlock()
	if err := s.Refresh(context.Background(), "L"); err != nil {
		t.Fatal(err)
	}
	if f.gets["exceptional-office.json.gz"] != 2 || f.gets["exceptional-home.json.gz"] != 1 {
		t.Fatalf("downloads = %v", f.gets)
	}
	if r := s.Results(); len(r) != 1 || r[0].ValueEx != 9 {
		t.Fatalf("results = %+v", r)
	}

	// With GitHub down the files already here are still used.
	s.checked = s.checked.Add(-2 * time.Hour)
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	if err := s.Refresh(context.Background(), "L"); err == nil {
		t.Fatal("no error while down")
	}
	if r := s.Results(); len(r) != 1 || s.Status().Error == "" {
		t.Fatalf("cache lost while down: %+v %+v", r, s.Status())
	}
}

func pricesGz(t *testing.T, league string, at time.Time) []byte {
	t.Helper()
	snap := prices.Snapshot{SchemaVersion: prices.SchemaVersion, League: league, GeneratedAt: at,
		Rates:    prices.Rates{DivineEx: 490, ChaosEx: 64},
		Currency: []prices.CurrencyPrice{{Name: "Divine Orb", ValueEx: 490}}}
	raw, _ := json.Marshal(snap)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()
	return buf.Bytes()
}

func TestSharedPricesAreUsedOnlyWhenFreshAndForTheLeague(t *testing.T) {
	now := time.Now().UTC()
	f := &fakeRelease{gets: map[string]int{}, updated: map[string]time.Time{"prices.json.gz": now},
		files: map[string][]byte{"prices.json.gz": pricesGz(t, "Forbidden Rites", now.Add(-30*time.Minute))}}
	srv := f.server(t)
	defer srv.Close()
	s := newStore(t, srv)
	if err := s.Refresh(context.Background(), "Forbidden Rites"); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Prices("forbidden rites", 3*time.Hour)
	if err != nil || snap.Rates.DivineEx != 490 {
		t.Fatalf("fresh prices refused: %v", err)
	}
	if _, err := s.Prices("Forbidden Rites", 10*time.Minute); err == nil {
		t.Fatal("stale prices accepted")
	}
	if _, err := s.Prices("Standard", 3*time.Hour); err == nil {
		t.Fatal("another league's prices accepted")
	}
	// Exceptional merging ignores the prices file.
	if len(s.Results()) != 0 || s.Status().Files != 0 {
		t.Fatalf("prices counted as exceptional data: %+v", s.Status())
	}
}
