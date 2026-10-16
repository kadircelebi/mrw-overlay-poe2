package collector

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"poe2filter/internal/prices"
)

// cxCategories gives an item the exchange lists but no other source does
// the category the filter themes it by; unknown classes are currency.
var cxCategories = map[string]string{
	"StackableCurrency":            "currency",
	"UncutSkillGemStackable":       "uncutgems",
	"UncutSupportGemStackable":     "uncutgems",
	"UncutReservationGemStackable": "uncutgems",
	"SoulCore":                     "soulcores",
	"Omen":                         "ritual",
	"Support Skill Gem":            "lineagesupportgems",
	"MapFragment":                  "fragments",
	"PinnacleKeyStackable":         "fragments",
	"Breachstone":                  "fragments",
	"AtlasCurrency":                "fragments",
	"VaultKey":                     "vaultkeys",
	"Expedition2Logbooks":          "expedition",
	"IncubatorStackable":           "incursion",
}

// cxCheckSpread is how far another source may stray from the exchange
// before the difference is logged (kept for review, never shown).
const cxCheckSpread = 0.5

// MergeCXPrices puts the exchange's prices over have. An item keeps its
// category and trade id; one only the exchange lists is added. An item the
// exchange saw too few trades of keeps another source's price, with the
// exchange's markets attached. It returns the merged list and log lines for
// items another source priced far apart.
func MergeCXPrices(have []prices.CurrencyPrice, cx CXResult) ([]prices.CurrencyPrice, []string) {
	type diff struct {
		name      string
		was, now  float64
		magnitude float64
	}
	var diffs []diff
	seen := map[string]bool{}
	out := make([]prices.CurrencyPrice, 0, len(have)+len(cx.Items))
	for _, c := range have {
		key := strings.ToLower(c.Name)
		seen[key] = true
		p, ok := cx.Items[c.Name]
		if !ok {
			out = append(out, c)
			continue
		}
		if cx.Weak[c.Name] && c.ValueEx > 0 {
			c.Markets = p.Markets
			out = append(out, c)
			continue
		}
		if c.ValueEx > 0 && math.Abs(p.ValueEx/c.ValueEx-1) > cxCheckSpread {
			diffs = append(diffs, diff{c.Name, c.ValueEx, p.ValueEx, max(p.ValueEx, c.ValueEx)})
		}
		c.ValueEx, c.LowEx, c.Markets = p.ValueEx, p.LowEx, p.Markets
		out = append(out, c)
	}
	names := make([]string, 0, len(cx.Items))
	for name := range cx.Items {
		if !seen[strings.ToLower(name)] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		p := cx.Items[name]
		p.Category = cxCategories[cx.Classes[name]]
		if p.Category == "" {
			p.Category = "currency"
		}
		out = append(out, p)
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].magnitude > diffs[j].magnitude })
	var notes []string
	if len(diffs) > 0 {
		notes = append(notes, fmt.Sprintf("%d items differ from the previous source by more than %.0f%%", len(diffs), cxCheckSpread*100))
		for _, d := range diffs[:min(len(diffs), 8)] {
			notes = append(notes, fmt.Sprintf("  %s: %.2f ex → %.2f ex", d.name, d.was, d.now))
		}
	}
	return out, notes
}
