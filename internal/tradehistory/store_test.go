package tradehistory

import (
	"math"
	"testing"
	"time"

	"poe2filter/internal/trade"
)

func sale(id string, at time.Time, amount float64, currency string) trade.HistorySale {
	return trade.HistorySale{ItemID: id, Time: at, Amount: amount, Currency: currency}
}

func TestMergeKeepsEverySaleOnceNewestFirst(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s := New(dir)
	first := []trade.HistorySale{sale("a", t0, 2, "divine"), sale("b", t0.Add(-time.Hour), 40, "chaos")}
	if n, err := s.Merge("L", first, t0); err != nil || n != 2 {
		t.Fatalf("first merge: %d %v", n, err)
	}
	// The site's next answer overlaps the last one; the same item sold
	// again later is a new sale.
	second := []trade.HistorySale{sale("a", t0.Add(2*time.Hour), 3, "divine"), sale("a", t0, 2, "divine")}
	if n, _ := s.Merge("L", second, t0.Add(2*time.Hour)); n != 1 {
		t.Fatalf("second merge added %d, want 1", n)
	}
	got := New(dir).Sales("L") // read back from the file
	if len(got) != 3 || !got[0].Time.Equal(t0.Add(2*time.Hour)) || got[2].ItemID != "b" {
		t.Fatalf("sales %+v", got)
	}
	if len(New(dir).Sales("Other")) != 0 {
		t.Fatal("leagues are kept apart")
	}
	if !New(dir).Fetched("L").Equal(t0.Add(2 * time.Hour)) {
		t.Fatal("fetch time not kept")
	}
}

func TestSumConvertsAtTodaysRatesAndKeepsUnknownCurrencies(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	sales := []trade.HistorySale{
		sale("a", t0, 2, "divine"),
		sale("b", t0.Add(-time.Hour), 40, "chaos"),
		sale("c", t0.Add(-2*time.Hour), 5, "mystery"),
		sale("d", t0.Add(-48*time.Hour), 1, "divine"),
	}
	rates := map[string]float64{"divine": 700, "chaos": 20, "exalted": 1}
	toEx := func(amount float64, cur string) (float64, bool) {
		r, ok := rates[cur]
		return amount * r, ok
	}
	all := Sum(sales, time.Time{}, toEx)
	// 2×700 + 40×20 + 1×700
	if all.Count != 4 || math.Abs(all.ValueEx-2900) > 1e-9 || all.Unpriced["mystery"] != 5 {
		t.Fatalf("all %+v", all)
	}
	recent := Sum(sales, t0.Add(-90*time.Minute), toEx)
	if recent.Count != 2 || math.Abs(recent.ValueEx-2200) > 1e-9 || recent.Unpriced != nil {
		t.Fatalf("recent %+v", recent)
	}
}
