//go:build live

// Live check against the real trade API. Uses a few searches of the IP quota.
//
//	go test -tags live -run TestLiveScan -v ./internal/trade -snapshot=path/to/prices.json
package trade

import (
	"context"
	"flag"
	"path/filepath"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

var snapshotPath = flag.String("snapshot", "", "prices.json with current rates")

func TestLiveScan(t *testing.T) {
	market, err := prices.Load(*snapshotPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	client := NewClient(market.League, 0.4)
	s := NewScanner(client, filepath.Join(t.TempDir(), "state.json"), func(m string) { t.Log(m) })
	s.SetMarket(market)
	s.SetCandidates([]Candidate{{Base: "Cavalry Boots"}, {Base: "Sekhema Sandals"}})

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	for i := 0; i < 4; i++ {
		s.mu.Lock()
		tg := s.next(time.Now())
		s.mu.Unlock()
		if tg == nil {
			break
		}
		start := time.Now()
		if err := s.scan(ctx, tg); err != nil {
			t.Fatalf("%s %s: %v", tg.base, tg.kind, err)
		}
		t.Logf("scanned %s %s (min %d) in %s", tg.base, tg.kind, tg.min, time.Since(start).Round(time.Second))
	}
	for _, r := range s.Results() {
		t.Logf("RESULT %-16s %-8s min=%d class=%-7s value=%.1f ex listings=%d samples=%d",
			r.Base, r.Kind, r.Min, r.Class, r.ValueEx, r.Listings, r.Samples)
	}
}

func TestLiveEvaluateMageblood(t *testing.T) {
	client := NewClient("Forbidden Rites", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := client.Evaluate(ctx, EvaluateRequest{
		Name: "Mageblood", BaseType: "Utility Belt", Rarity: "unique", Status: "any",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SearchID == "" || result.Total == 0 || len(result.Listings) == 0 {
		t.Fatalf("empty result: %+v", result)
	}
	t.Logf("search=%s total=%d fetched=%d first=%g %s", result.SearchID, result.Total, len(result.Listings), result.Listings[0].Amount, result.Listings[0].Currency)
}
