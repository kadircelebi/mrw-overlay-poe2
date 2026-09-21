package collector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"poe2filter/internal/prices"
)

// poe.ninja splits its economy data in two: stash listings (uniques, handled in
// ninja.go) and the currency exchange, which covers everything that trades in
// bulk. poe2scout gives us most of the second group, but not all of it — the
// gaps have included whole lineage support gems and soul cores worth thousands
// of exalted, which then fell through to the base filter unpriced.
const ninjaExchangeURL = "https://poe.ninja/poe2/api/economy/exchange/current/overview"

// NinjaExchangeTypes are every exchange category poe.ninja actually serves for
// PoE2, checked one by one against the API (an unknown type answers 200 with an
// empty list, so a wrong name fails silently — see the warning in
// FetchNinjaExchange). "Ritual" holds the omens, "Delirium" the liquid
// emotions, "Breach" the catalysts and "Abyss" the bones.
//
// Uncut gems are left out on purpose: poe.ninja prices them per level ("Uncut
// Skill Gem (Level 20)"), which is not a base type, and the gem sliders already
// handle them by level.
var NinjaExchangeTypes = []string{
	"Currency",
	"Fragments",
	"Runes",
	"SoulCores",
	"Essences",
	"LineageSupportGems",
	"Idols",
	"Verisium",
	"Expedition",
	"Ritual",
	"Delirium",
	"Breach",
	"Abyss",
}

// errUnknownCategory marks a type poe.ninja answers with an empty list, which
// in practice means the name is no longer one of its categories.
var errUnknownCategory = errors.New("poe.ninja returned no prices for this category (renamed?)")

type ninjaExchangeResponse struct {
	Core struct {
		Rates map[string]float64 `json:"rates"` // units per 1 Divine Orb
	} `json:"core"`
	Items []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"items"`
	Lines []struct {
		ID           string  `json:"id"`
		PrimaryValue float64 `json:"primaryValue"` // in Divine Orbs
		// VolumePrimaryValue is how much traded recently; zero means nobody is
		// buying, so the price is a guess we should not hide an item over.
		VolumePrimaryValue float64 `json:"volumePrimaryValue"`
	} `json:"lines"`
}

// NinjaExchangePrice is one bulk item as poe.ninja reports it.
type NinjaExchangePrice struct {
	Name     string
	Category string
	ValueDiv float64
}

// FetchNinjaExchange returns the bulk prices of the given categories, keyed by
// nothing in particular: the caller decides how to merge them with poe2scout.
// A category that fails is reported in failed and simply contributes nothing.
// A category that answers with nothing at all is reported the same way: that
// means poe.ninja renamed it, which would otherwise go unnoticed.
func FetchNinjaExchange(ctx context.Context, c *http.Client, league string, types []string) (out []NinjaExchangePrice, divineEx float64, failed map[string]error) {
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	failed = map[string]error{}
	for _, t := range types {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			u := fmt.Sprintf("%s?league=%s&type=%s", ninjaExchangeURL, url.QueryEscape(league), url.QueryEscape(t))
			var resp ninjaExchangeResponse
			if err := getJSON(ctx, c, u, &resp); err != nil {
				mu.Lock()
				failed[t] = err
				mu.Unlock()
				return
			}
			names := make(map[string]string, len(resp.Items))
			for _, it := range resp.Items {
				names[it.ID] = strings.TrimSpace(it.Name)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(resp.Lines) == 0 {
				failed[t] = errUnknownCategory
				return
			}
			if r := resp.Core.Rates["exalted"]; r > divineEx {
				divineEx = r
			}
			for _, l := range resp.Lines {
				name := names[l.ID]
				if name == "" || l.PrimaryValue <= 0 {
					continue
				}
				out = append(out, NinjaExchangePrice{
					Name:     name,
					Category: strings.ToLower(t),
					ValueDiv: l.PrimaryValue,
				})
			}
		}(t)
	}
	wg.Wait()
	return out, divineEx, failed
}

// MergeExchangePrices adds the poe.ninja prices that poe2scout does not carry.
// poe2scout stays the primary source — it is what the thresholds were tuned
// against — so this only ever fills holes, never overwrites a price.
func MergeExchangePrices(have []prices.CurrencyPrice, add []NinjaExchangePrice, divineEx float64) []prices.CurrencyPrice {
	if divineEx <= 0 {
		return have
	}
	known := make(map[string]bool, len(have))
	for _, c := range have {
		known[strings.ToLower(c.Name)] = true
	}
	for _, p := range add {
		key := strings.ToLower(p.Name)
		if known[key] {
			continue
		}
		known[key] = true
		have = append(have, prices.CurrencyPrice{
			Name:     p.Name,
			Category: p.Category,
			ValueEx:  p.ValueDiv * divineEx,
		})
	}
	return have
}
