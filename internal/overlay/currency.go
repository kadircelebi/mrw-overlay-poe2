package overlay

import (
	"strings"
	"time"

	"poe2filter/internal/prices"
)

// CurrencyQuote is what the overlay shows for a stackable item: its value in
// the price snapshot the filter already uses, so no trade search is needed.
type CurrencyQuote struct {
	Found       bool      `json:"found"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	ValueEx     float64   `json:"valueEx"`
	DivineEx    float64   `json:"divineEx"`
	ChaosEx     float64   `json:"chaosEx"`
	League      string    `json:"league"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// QuoteCurrency looks the name up in the snapshot. The three rate currencies
// are answered from the rates even when the list does not carry them.
func QuoteCurrency(snap *prices.Snapshot, name string) CurrencyQuote {
	name = strings.TrimSpace(name)
	q := CurrencyQuote{Name: name}
	if snap == nil || name == "" {
		return q
	}
	// Convert with the list's own Divine/Chaos prices when it has them: the
	// listed values come from the same source, while Rates may come from
	// another one and would mix two markets in one figure.
	q.DivineEx, q.ChaosEx = snap.Rates.DivineEx, snap.Rates.ChaosEx
	for _, c := range snap.Currency {
		switch {
		case c.ValueEx <= 0:
		case strings.EqualFold(c.Name, "Divine Orb"):
			q.DivineEx = c.ValueEx
		case strings.EqualFold(c.Name, "Chaos Orb"):
			q.ChaosEx = c.ValueEx
		}
	}
	q.League, q.GeneratedAt = snap.League, snap.GeneratedAt
	for _, c := range snap.Currency {
		if strings.EqualFold(c.Name, name) && c.ValueEx > 0 {
			q.Found, q.Name, q.Category, q.ValueEx = true, c.Name, c.Category, c.ValueEx
			return q
		}
	}
	switch strings.ToLower(name) {
	case "exalted orb":
		q.Found, q.ValueEx = true, 1
	case "divine orb":
		q.Found, q.ValueEx = q.DivineEx > 0, q.DivineEx
	case "chaos orb":
		q.Found, q.ValueEx = q.ChaosEx > 0, q.ChaosEx
	}
	if q.Found {
		q.Category = "Currency"
	}
	return q
}
