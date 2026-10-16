package collector

import (
	"math"
	"slices"
	"sort"

	"poe2filter/internal/prices"
)

// Pricing from GGG's Currency Exchange digests: completed trades per market
// and hour. A market pairs an item with the currency it was traded against;
// only Exalted, Chaos and Divine count as such currencies ("bases").
//
// For an item X and a base B, the price of one X is
//
//	units of B paid / units of X traded   (summed over the window's hours)
//
// in B, times B's rate in Exalted Orbs. The window starts at the newest hour
// and grows hour by hour until cxEnoughUnits of X have traded on that market
// or cxWindowHours are used, so a liquid market follows the last hour and a
// thin one gathers enough trades.
//
// A market counts only if it saw enough of the item trade: at least
// cxMinUnits units and cxMinShare of all the item's units over its markets.
// A single 1:1 trade against a Chaos or Divine Orb would otherwise set the
// price of an item that trades for an Exalted Orb all day. When no market is
// that busy, the busiest one is used.
//
// Markets for one item can disagree (sellers bundle cheap items and ask a
// Divine, a buyer with no Exalted pays it). When the highest market is
// within cxSpread of the lowest, the item's price is the unit-weighted mean
// of its markets (total Exalted paid / total units). Otherwise the markets
// are kept apart: the Exalted market is the price when at least cxMinUnits
// traded on it (everyday trade, and the way trade starts in a new league);
// without such an Exalted market the price is the highest market that saw
// enough trades (optimistic: an item that sold that dear is not hidden).
// The lowest market is kept as LowEx when it is below the price.
const (
	cxWindowHours = 6
	cxEnoughUnits = 10.0
	cxSpread      = 3.0
	cxMinUnits    = 5.0
	cxMinShare    = 0.2
)

// Trade ids of the bases.
const (
	cxExalted = "Metadata/Items/Currency/CurrencyAddModToRare"
	cxChaos   = "Metadata/Items/Currency/CurrencyRerollRare"
	cxDivine  = "Metadata/Items/Currency/CurrencyModValues"
)

var cxBases = map[string]string{cxExalted: "exalted", cxChaos: "chaos", cxDivine: "divine"}

// cxTrade is one market in one hour: the units of A and of B traded.
type cxTrade struct {
	A, B   string
	QA, QB float64
}

// cxVolume is a market's units over its window: Item units of the item,
// Base units of the base paid for them.
type cxVolume struct {
	Item, Base float64
	Hours      int
}

type cxPair struct{ item, base string }

// cxVolumes sums every (item, base) market over its window. hours is newest
// first; each element holds one hour's markets of one league.
func cxVolumes(hours [][]cxTrade) map[cxPair]cxVolume {
	out := map[cxPair]cxVolume{}
	done := map[cxPair]bool{}
	for i, hour := range hours {
		if i >= cxWindowHours {
			break
		}
		seen := map[cxPair]bool{}
		for _, t := range hour {
			for _, side := range [2]cxTrade{t, {A: t.B, B: t.A, QA: t.QB, QB: t.QA}} {
				if _, ok := cxBases[side.B]; !ok || side.A == side.B || side.QA <= 0 || side.QB <= 0 {
					continue
				}
				k := cxPair{side.A, side.B}
				if done[k] {
					continue
				}
				v := out[k]
				v.Item += side.QA
				v.Base += side.QB
				if !seen[k] {
					v.Hours++
					seen[k] = true
				}
				out[k] = v
			}
		}
		// Whole hours only: a market that reached enough units stops here.
		for k, v := range out {
			if v.Item >= cxEnoughUnits {
				done[k] = true
			}
		}
	}
	return out
}

// cxRates are the bases' worth in Exalted Orbs. The Chaos rate falls back
// to its Divine market when it saw no trade against Exalted.
func cxRates(vol map[cxPair]cxVolume) prices.Rates {
	var r prices.Rates
	if v, ok := vol[cxPair{cxDivine, cxExalted}]; ok {
		r.DivineEx = v.Base / v.Item
	}
	if v, ok := vol[cxPair{cxChaos, cxExalted}]; ok {
		r.ChaosEx = v.Base / v.Item
	} else if v, ok := vol[cxPair{cxChaos, cxDivine}]; ok && r.DivineEx > 0 {
		r.ChaosEx = v.Base / v.Item * r.DivineEx
	}
	return r
}

// cxItemPrices prices every item that traded against a base, keyed by trade
// id. Exalted is 1 by definition; Divine and Chaos are their rates.
func cxItemPrices(vol map[cxPair]cxVolume, rates prices.Rates) map[string]prices.CurrencyPrice {
	out, _ := cxItemPricesWeak(vol, rates)
	return out
}

// cxItemPricesWeak also returns the items whose markets all saw fewer than
// cxMinUnits units: their price rests on one or two trades, so another
// source is preferred when it has one.
func cxItemPricesWeak(vol map[cxPair]cxVolume, rates prices.Rates) (map[string]prices.CurrencyPrice, map[string]bool) {
	baseEx := map[string]float64{cxExalted: 1, cxChaos: rates.ChaosEx, cxDivine: rates.DivineEx}
	markets := map[string][]prices.MarketPrice{}
	for k, v := range vol {
		if _, isBase := cxBases[k.item]; isBase || baseEx[k.base] <= 0 {
			continue
		}
		markets[k.item] = append(markets[k.item], prices.MarketPrice{
			Base:    cxBases[k.base],
			ValueEx: v.Base / v.Item * baseEx[k.base],
			Units:   v.Item,
			Hours:   v.Hours,
		})
	}
	out := make(map[string]prices.CurrencyPrice, len(markets)+3)
	weak := map[string]bool{}
	for id, list := range markets {
		sort.Slice(list, func(i, j int) bool { return list[i].Base < list[j].Base })
		out[id] = cxCombine(list)
		if !slices.ContainsFunc(list, func(m prices.MarketPrice) bool { return m.Units >= cxMinUnits }) {
			weak[id] = true
		}
	}
	out[cxExalted] = prices.CurrencyPrice{ValueEx: 1}
	if rates.DivineEx > 0 {
		out[cxDivine] = prices.CurrencyPrice{ValueEx: rates.DivineEx}
	}
	if rates.ChaosEx > 0 {
		out[cxChaos] = prices.CurrencyPrice{ValueEx: rates.ChaosEx}
	}
	return out, weak
}

// cxCombine turns an item's markets into its price (see the notes above the
// constants). Markets keeps every market, the price uses the busy ones.
func cxCombine(list []prices.MarketPrice) prices.CurrencyPrice {
	busy := cxBusy(list)
	var exalted *prices.MarketPrice
	for i := range list {
		if list[i].Base == "exalted" && list[i].Units >= cxMinUnits {
			exalted = &list[i]
		}
	}
	lo, hi := math.Inf(1), 0.0
	var paid, units float64
	for _, m := range busy {
		lo, hi = min(lo, m.ValueEx), max(hi, m.ValueEx)
		paid += m.ValueEx * m.Units
		units += m.Units
	}
	if exalted != nil {
		lo, hi = min(lo, exalted.ValueEx), max(hi, exalted.ValueEx)
	}
	p := prices.CurrencyPrice{Markets: list}
	switch {
	case hi <= lo*cxSpread:
		p.ValueEx = paid / units
		if exalted != nil && !slices.ContainsFunc(busy, func(m prices.MarketPrice) bool { return m.Base == "exalted" }) {
			p.ValueEx = (paid + exalted.ValueEx*exalted.Units) / (units + exalted.Units)
		}
	case exalted != nil:
		p.ValueEx = exalted.ValueEx
	default:
		p.ValueEx = hi
	}
	if lo < p.ValueEx*(1-1e-9) && hi > lo*cxSpread {
		p.LowEx = lo
	}
	return p
}

// cxBusy keeps the markets that saw enough of the item trade, or the
// busiest one when none did.
func cxBusy(list []prices.MarketPrice) []prices.MarketPrice {
	var total float64
	busiest := list[0]
	for _, m := range list {
		total += m.Units
		if m.Units > busiest.Units {
			busiest = m
		}
	}
	var keep []prices.MarketPrice
	for _, m := range list {
		if m.Units >= cxMinUnits && m.Units >= cxMinShare*total {
			keep = append(keep, m)
		}
	}
	if len(keep) == 0 {
		return []prices.MarketPrice{busiest}
	}
	return keep
}
