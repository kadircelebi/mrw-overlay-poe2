package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

func TestShardsSplitEveryKeyOnceByWeight(t *testing.T) {
	weights, err := ParseShardWeights("office=60, home=40")
	if err != nil {
		t.Fatal(err)
	}
	office, _ := ShardFilter("office", weights)
	home, _ := ShardFilter("home", weights)
	n, officeKeys := 5000, 0
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("Base %d|quality|ilvl82+", i)
		a, b := office(key), home(key)
		if a == b {
			t.Fatalf("%s: office=%v home=%v, want exactly one", key, a, b)
		}
		if a {
			officeKeys++
		}
	}
	if share := float64(officeKeys) / float64(n); share < 0.56 || share > 0.64 {
		t.Fatalf("office share %.2f, want about 0.60", share)
	}
	// The split never depends on the order the weights were written in.
	again, _ := ShardFilter("office", map[string]int{"home": 40, "office": 60})
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("K%d", i)
		if again(key) != office(key) {
			t.Fatal("split changed with the weight order")
		}
	}
}

func TestShardWeightsAreChecked(t *testing.T) {
	for _, bad := range []string{"", "office", "office=0", "office=x", "office=1,office=2"} {
		if _, err := ParseShardWeights(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := ShardFilter("lab", map[string]int{"office": 1}); err == nil {
		t.Error("unknown shard accepted")
	}
}

// scanAPI records search bodies and answers one normal item per search.
type scanAPI struct {
	mu     sync.Mutex
	bodies []string
}

func (a *scanAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	reply := func(body string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}
	if strings.Contains(req.URL.Path, "/search/") {
		raw, _ := io.ReadAll(req.Body)
		a.mu.Lock()
		a.bodies = append(a.bodies, string(raw))
		a.mu.Unlock()
		return reply(`{"id":"S1","result":["00aa11bb22cc33dd"],"total":1}`)
	}
	return reply(`{"result":[{"listing":{"price":{"amount":40,"currency":"exalted"}},"item":{"baseType":"Vaal Cuirass","rarity":"Normal","ilvl":82,"properties":[{"name":"Body Armour","values":[]},{"name":"Quality","values":[["+25%",1]]}]}}]}`)
}

func TestIlvlBucketsAreKeysOfTheirOwn(t *testing.T) {
	api := &scanAPI{}
	client := NewInteractiveClient("Forbidden Rites", 1)
	client.http.Transport = api
	s := NewScanner(client, filepath.Join(t.TempDir(), "scan.json"), nil)
	s.SetMarket(&prices.Snapshot{Rates: prices.Rates{DivineEx: 400}})
	s.SetCandidates([]Candidate{{Base: "Vaal Cuirass"}})
	s.SetIlvlBuckets([]IlvlBucket{{Min: 79, Max: 81}, {Min: 82}})
	// The trade site names the class "Body Armour"; it must still be read
	// as Body Armours (3+ sockets exceptional) after the first result.
	s.mu.Lock()
	s.st.Classes["Vaal Cuirass"] = "Body Armours"
	s.mu.Unlock()

	for i := 0; i < 4; i++ {
		s.mu.Lock()
		target := s.next(time.Now())
		s.mu.Unlock()
		if target == nil {
			t.Fatalf("only %d targets, want 4 (2 kinds × 2 buckets)", i)
		}
		if err := s.scan(context.Background(), target); err != nil {
			t.Fatal(err)
		}
	}
	s.mu.Lock()
	extra := s.next(time.Now())
	s.mu.Unlock()
	if extra != nil {
		t.Fatalf("fifth target %s", extra.key)
	}

	want := map[string][2]int{
		"Vaal Cuirass|quality|ilvl79-81": {79, 81}, "Vaal Cuirass|quality|ilvl82+": {82, 0},
		"Vaal Cuirass|sockets|ilvl79-81": {79, 81}, "Vaal Cuirass|sockets|ilvl82+": {82, 0},
	}
	for key, bounds := range want {
		ks := s.st.Keys[key]
		if ks == nil || ks.ScannedAt.IsZero() || ks.MinIlvl != bounds[0] || ks.MaxIlvl != bounds[1] {
			t.Fatalf("%s = %+v", key, ks)
		}
	}
	// Every search carried its item level range.
	var ranges []string
	for _, body := range api.bodies {
		var q struct {
			Query struct {
				Filters map[string]struct {
					Filters map[string]struct {
						Min *int `json:"min"`
						Max *int `json:"max"`
					} `json:"filters"`
				} `json:"filters"`
			} `json:"query"`
		}
		if err := json.Unmarshal([]byte(body), &q); err != nil {
			t.Fatal(err)
		}
		r := q.Query.Filters["type_filters"].Filters["ilvl"]
		got := "open"
		if r.Min != nil {
			got = fmt.Sprint(*r.Min, "-")
			if r.Max != nil {
				got += fmt.Sprint(*r.Max)
			}
		}
		ranges = append(ranges, got)
	}
	if strings.Count(strings.Join(ranges, ","), "79-81") != 2 || strings.Count(strings.Join(ranges, ","), "82-") != 2 {
		t.Fatalf("search ranges %v", ranges)
	}
}

func TestShardedScannerOnlySearchesItsKeys(t *testing.T) {
	client := NewClient("Forbidden Rites", 1)
	s := NewScanner(client, filepath.Join(t.TempDir(), "scan.json"), nil)
	s.SetCandidates([]Candidate{{Base: "A"}, {Base: "B"}, {Base: "C"}, {Base: "D"}, {Base: "E"}})
	s.SetIlvlBuckets([]IlvlBucket{{Min: 79, Max: 81}, {Min: 82}})
	weights := map[string]int{"office": 60, "home": 40}
	office, _ := ShardFilter("office", weights)
	home, _ := ShardFilter("home", weights)

	s.SetShard(office)
	a := s.Status().Keys
	s.SetShard(home)
	b := s.Status().Keys
	s.SetShard(nil)
	all := s.Status().Keys
	if all != 20 || a+b != all || a == 0 || b == 0 {
		t.Fatalf("office %d + home %d != all %d", a, b, all)
	}
	s.SetShard(home)
	s.mu.Lock()
	target := s.next(time.Now())
	s.mu.Unlock()
	if target == nil || !home(target.key) {
		t.Fatalf("home scanner picked %+v", target)
	}
}

func TestTradeClassNamesMatchTheGame(t *testing.T) {
	for in, want := range map[string]string{
		"Body Armour": "Body Armours", "Helmet": "Helmets", "Boots": "Boots", "Gloves": "Gloves",
		"[Shield]": "Shields", "[Mace|Two Hand Mace]": "Two Hand Maces", "[Quarterstaff]": "Quarterstaves",
		"[Focus]": "Foci", "[Staff]": "Staves", "Body Armours": "Body Armours",
	} {
		got := NormalizeClass(in)
		if got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
		if ExceptionalSocketMin(got) == 0 && ExceptionalSocketMin(want) != 0 {
			t.Errorf("%q lost its socket rule", in)
		}
	}
}
