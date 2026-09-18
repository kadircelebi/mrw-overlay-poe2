// Package insights prepares price summaries and search lists for the UI.
package insights

import (
	"sort"
	"strings"

	"poe2filter/internal/prices"
)

// TopUniqueItem for GUI analytics.
type TopUniqueItem struct {
	Name     string  `json:"name"`
	Base     string  `json:"base"`
	Divine   float64 `json:"divine"`
	Exalt    float64 `json:"exalt"`
	Category string  `json:"category"`
}

// TopScoutItem for GUI analytics.
type TopScoutItem struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Exalt    float64 `json:"exalt"`
	Divine   float64 `json:"divine"`
}

// TopExceptionalItem for GUI analytics.
type TopExceptionalItem struct {
	Base     string  `json:"base"`
	Kind     string  `json:"kind"`
	Min      int     `json:"min"`
	Exalt    float64 `json:"exalt"`
	Listings int     `json:"listings"`
}

// MarketInsights contains top ranked items for display.
type MarketInsights struct {
	Timestamp      string               `json:"timestamp"`
	TopUniques     []TopUniqueItem      `json:"top_uniques"`
	TopScout       []TopScoutItem       `json:"top_scout"`
	TopExceptional []TopExceptionalItem `json:"top_exceptional"`
}

// Get returns the most valuable uniques, bulk items and exceptional bases.
func Get(s *prices.Snapshot) MarketInsights {
	out := MarketInsights{
		Timestamp:      s.GeneratedAt.Local().Format("2006-01-02 15:04"),
		TopUniques:     []TopUniqueItem{},
		TopScout:       []TopScoutItem{},
		TopExceptional: []TopExceptionalItem{},
	}
	div := s.Rates.DivineEx

	var uniques []TopUniqueItem
	for base, ub := range s.UniqueBases {
		for _, u := range ub.Uniques {
			uniques = append(uniques, TopUniqueItem{Name: u.Name, Base: base, Exalt: u.ValueEx,
				Divine: u.ValueEx / div, Category: strings.TrimPrefix(u.Category, "Unique")})
		}
	}
	sort.Slice(uniques, func(i, j int) bool { return uniques[i].Exalt > uniques[j].Exalt })
	out.TopUniques = append(out.TopUniques, uniques[:min(10, len(uniques))]...)

	cur := append([]prices.CurrencyPrice(nil), s.Currency...)
	sort.Slice(cur, func(i, j int) bool { return cur[i].ValueEx > cur[j].ValueEx })
	for _, c := range cur[:min(10, len(cur))] {
		out.TopScout = append(out.TopScout, TopScoutItem{Name: c.Name, Category: c.Category,
			Exalt: c.ValueEx, Divine: c.ValueEx / div})
	}

	ex := append([]prices.ExceptionalPrice(nil), s.Exceptional...)
	sort.Slice(ex, func(i, j int) bool { return ex[i].ValueEx > ex[j].ValueEx })
	for _, e := range ex[:min(10, len(ex))] {
		if e.Samples == 0 {
			continue
		}
		out.TopExceptional = append(out.TopExceptional, TopExceptionalItem{Base: e.Base, Kind: string(e.Kind),
			Min: e.Min, Exalt: e.ValueEx, Listings: e.Listings})
	}
	return out
}

// SearchItem represents an item or base available for search in custom lists.
type SearchItem struct {
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Type           string   `json:"type"` // "unique", "currency", "base", ...
	PriceDivine    float64  `json:"price_divine,omitempty"`
	PriceExalt     float64  `json:"price_exalt,omitempty"`
	BaseType       string   `json:"base_type,omitempty"`
	RelatedUniques []string `json:"related_uniques,omitempty"`
}

// SearchItems lists every unique, bulk item and base the user can put on the
// whitelist, blacklist or chance list. validBases comes from the base filter.
func SearchItems(s *prices.Snapshot, validBases map[string]string) []SearchItem {
	seen := map[string]bool{}
	var out []SearchItem
	add := func(it SearchItem) {
		k := strings.ToLower(it.Name)
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, it)
	}
	div := 1.0
	if s != nil && s.Rates.DivineEx > 0 {
		div = s.Rates.DivineEx
	}

	baseUniques := map[string][]string{}
	if s != nil {
		for base, ub := range s.UniqueBases {
			for _, u := range ub.Uniques {
				baseUniques[strings.ToLower(base)] = append(baseUniques[strings.ToLower(base)], u.Name)
				add(SearchItem{Name: u.Name, Category: strings.TrimPrefix(u.Category, "Unique") + " (Unique)",
					Type: "unique", PriceExalt: u.ValueEx, PriceDivine: u.ValueEx / div, BaseType: base})
			}
		}
		for _, c := range s.Currency {
			add(SearchItem{Name: c.Name, Category: title(c.Category),
				Type: c.Category, PriceExalt: c.ValueEx, PriceDivine: c.ValueEx / div})
		}
	}
	for _, base := range validBases {
		related := baseUniques[strings.ToLower(base)]
		cat := "Crafting Tabanı"
		var maxEx float64
		if s != nil {
			if ub, ok := s.UniqueBases[base]; ok {
				cat, maxEx = "Chance & Crafting Tabanı", ub.MaxEx
			}
		}
		add(SearchItem{Name: base, Category: cat, Type: "base", BaseType: base,
			PriceExalt: maxEx, PriceDivine: maxEx / div, RelatedUniques: related})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func title(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
