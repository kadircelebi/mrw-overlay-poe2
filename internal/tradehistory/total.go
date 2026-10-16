package tradehistory

import (
	"time"

	"poe2filter/internal/trade"
)

// Total sums sales at today's rates. A sale priced in a currency without a
// rate is not guessed: its amount is kept under Unpriced by currency.
type Total struct {
	Count    int                `json:"count"`
	ValueEx  float64            `json:"valueEx"`
	Unpriced map[string]float64 `json:"unpriced,omitempty"`
}

// Sum totals the sales made at or after since (all of them when since is
// zero). toEx converts an amount of a trade currency id into Exalted Orbs.
//
//	ValueEx = Σ amount × (that currency's worth in Exalted Orbs today)
func Sum(sales []trade.HistorySale, since time.Time, toEx func(amount float64, currency string) (float64, bool)) Total {
	var t Total
	for _, s := range sales {
		if !since.IsZero() && s.Time.Before(since) {
			continue
		}
		t.Count++
		if ex, ok := toEx(s.Amount, s.Currency); ok {
			t.ValueEx += ex
			continue
		}
		if t.Unpriced == nil {
			t.Unpriced = map[string]float64{}
		}
		t.Unpriced[s.Currency] += s.Amount
	}
	return t
}
