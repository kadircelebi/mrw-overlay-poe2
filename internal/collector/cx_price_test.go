package collector

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9*max(1, math.Abs(b)) }

const (
	tasalio = "Metadata/Items/Currency/Tasalio"
	mirror  = "Metadata/Items/Currency/CurrencyDuplicate"
	thin    = "Metadata/Items/Currency/Thin"
)

func TestCXRatesAndPrices(t *testing.T) {
	hours := [][]cxTrade{
		// newest hour
		{
			{A: cxDivine, B: cxExalted, QA: 20, QB: 15000}, // 750 ex/div
			{A: cxExalted, B: cxChaos, QA: 700, QB: 10},    // 70 ex/chaos, written either way round
			{A: mirror, B: cxDivine, QA: 1, QB: 5000},
			{A: thin, B: cxExalted, QA: 3, QB: 6},
		},
		{
			{A: thin, B: cxExalted, QA: 3, QB: 3},
			{A: mirror, B: cxDivine, QA: 1, QB: 5200},
		},
	}
	vol := cxVolumes(hours)
	r := cxRates(vol)
	if !near(r.DivineEx, 750) || !near(r.ChaosEx, 70) {
		t.Fatalf("rates %+v", r)
	}
	p := cxItemPrices(vol, r)
	// Mirror: 2 traded for 10200 div over two hours = 5100 div = 3825000 ex.
	if m := p[mirror]; !near(m.ValueEx, 5100*750) || m.Markets[0].Hours != 2 {
		t.Errorf("mirror %+v", m)
	}
	// Thin: 6 units for 9 ex = 1.5 ex.
	if !near(p[thin].ValueEx, 1.5) {
		t.Errorf("thin %+v", p[thin])
	}
	if !near(p[cxExalted].ValueEx, 1) || !near(p[cxDivine].ValueEx, 750) || !near(p[cxChaos].ValueEx, 70) {
		t.Errorf("bases %v %v %v", p[cxExalted], p[cxDivine], p[cxChaos])
	}
}

// A market that reached cxEnoughUnits stops gathering older hours, so a
// liquid market follows the newest hour.
func TestCXWindowStopsWhenEnough(t *testing.T) {
	hours := [][]cxTrade{
		{{A: cxDivine, B: cxExalted, QA: 12, QB: 12 * 800}},
		{{A: cxDivine, B: cxExalted, QA: 50, QB: 50 * 600}},
	}
	if r := cxRates(cxVolumes(hours)); !near(r.DivineEx, 800) {
		t.Fatalf("divine %v, want the newest hour's 800", r.DivineEx)
	}
	// Seven hours of one unit each: only cxWindowHours are used.
	var long [][]cxTrade
	for i := 0; i < 7; i++ {
		long = append(long, []cxTrade{{A: thin, B: cxExalted, QA: 1, QB: float64(i + 1)}})
	}
	if v := cxVolumes(long)[cxPair{thin, cxExalted}]; v.Hours != cxWindowHours || !near(v.Base, 21) {
		t.Fatalf("window %+v", v)
	}
	// A market with no completed trade does not count.
	if _, ok := cxVolumes([][]cxTrade{{{A: thin, B: cxExalted, QA: 0, QB: 0}}})[cxPair{thin, cxExalted}]; ok {
		t.Fatal("empty market counted")
	}
}

// Markets within three times of each other are averaged by units; farther
// apart, the highest one that traded is the price and the lowest is kept.
func TestCXCombineMarkets(t *testing.T) {
	r := cxRates(cxVolumes([][]cxTrade{{
		{A: cxDivine, B: cxExalted, QA: 10, QB: 7500},
		{A: cxChaos, B: cxExalted, QA: 10, QB: 700},
	}}))
	agree := cxItemPrices(cxVolumes([][]cxTrade{{
		{A: thin, B: cxExalted, QA: 30, QB: 60}, // 2 ex
		{A: thin, B: cxChaos, QA: 70, QB: 3},    // 3 ex each
	}}), r)[thin]
	// (30*2 + 70*3) / 100 = 2.7
	if !near(agree.ValueEx, 2.7) || agree.LowEx != 0 || len(agree.Markets) != 2 {
		t.Errorf("agreeing markets %+v", agree)
	}
	// Tasalio: 1 ex on Exalted, 5 for a Divine (150 ex) on Divine; both
	// busy enough (8 and 5 of 13).
	apart := cxItemPrices(cxVolumes([][]cxTrade{{
		{A: tasalio, B: cxExalted, QA: 8, QB: 8},
		{A: tasalio, B: cxDivine, QA: 5, QB: 1},
	}}), r)[tasalio]
	// The Exalted market traded enough (8 >= 5): it is the price.
	if !near(apart.ValueEx, 1) || apart.LowEx != 0 {
		t.Errorf("disagreeing markets %+v", apart)
	}
	// Without a busy Exalted market the highest busy market is the price.
	noEx := cxItemPrices(cxVolumes([][]cxTrade{{
		{A: tasalio, B: cxChaos, QA: 10, QB: 5},   // 35 ex each
		{A: tasalio, B: cxDivine, QA: 10, QB: 10}, // 750 ex each
		{A: tasalio, B: cxExalted, QA: 3, QB: 3},
	}}), r)[tasalio]
	if !near(noEx.ValueEx, 750) || !near(noEx.LowEx, 35) {
		t.Errorf("no busy Exalted market %+v", noEx)
	}
}

// A lone 1:1 trade against a Chaos Orb does not price an item that trades
// for an Exalted Orb all day; a busy bulk market against Divine does count.
func TestCXThinMarketsDoNotCount(t *testing.T) {
	r := cxRates(cxVolumes([][]cxTrade{{
		{A: cxDivine, B: cxExalted, QA: 10, QB: 7000},
		{A: cxChaos, B: cxExalted, QA: 10, QB: 700},
	}}))
	lone := cxItemPrices(cxVolumes([][]cxTrade{{
		{A: thin, B: cxExalted, QA: 50, QB: 450}, // 9 ex
		{A: thin, B: cxChaos, QA: 1, QB: 1},      // 70 ex, one trade
	}}), r)[thin]
	if !near(lone.ValueEx, 9) || lone.LowEx != 0 || len(lone.Markets) != 2 {
		t.Errorf("lone trade counted: %+v", lone)
	}
	// 120 for 6 Divine (35 ex each) against 26 for 60 Exalted (2.3 ex):
	// apart, and 26 Exalted trades are enough, so the Exalted market prices
	// it; the bulk Divine market stays in Markets.
	bulk := cxItemPrices(cxVolumes([][]cxTrade{{
		{A: tasalio, B: cxDivine, QA: 120, QB: 6},
		{A: tasalio, B: cxExalted, QA: 26, QB: 60},
	}}), r)[tasalio]
	if !near(bulk.ValueEx, 60.0/26) || len(bulk.Markets) != 2 {
		t.Errorf("bulk market %+v", bulk)
	}
	// Nothing busy: the busiest market.
	quiet := cxItemPrices(cxVolumes([][]cxTrade{{
		{A: thin, B: cxExalted, QA: 3, QB: 6},
		{A: thin, B: cxChaos, QA: 1, QB: 1},
	}}), r)[thin]
	if !near(quiet.ValueEx, 2) {
		t.Errorf("quiet item %+v", quiet)
	}
}

// An item no market saw five of is weak: its price rests on a trade or two.
func TestCXWeakItems(t *testing.T) {
	vol := cxVolumes([][]cxTrade{{
		{A: cxDivine, B: cxExalted, QA: 10, QB: 7500},
		{A: thin, B: cxExalted, QA: 3, QB: 6},
		{A: mirror, B: cxDivine, QA: 5, QB: 25000},
	}})
	_, weak := cxItemPricesWeak(vol, cxRates(vol))
	if !weak[thin] || weak[mirror] {
		t.Fatalf("weak %v", weak)
	}
}
