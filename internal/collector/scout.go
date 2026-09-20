package collector

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"poe2filter/internal/prices"
)

const scoutAPIURL = "https://api.poe2scout.com/poe2/Leagues/%s/Items"

type scoutItem struct {
	CategoryApiId string  `json:"CategoryApiId"`
	Text          string  `json:"Text"`
	Name          *string `json:"Name"`
	Type          *string `json:"Type"`
	ApiId         string  `json:"ApiId"`
	CurrentPrice  float64 `json:"CurrentPrice"` // in Exalted Orbs
}

// scoutSkipCategories are handled elsewhere: equipment uniques come from
// poe.ninja, waystones and uncut gems have dedicated tier rules.
var scoutSkipCategories = map[string]bool{
	"accessory": true, "armour": true, "weapon": true, "flask": true,
	"jewel": true, "sanctum": true, "talismans": true, "map": true,
	"waystones": true, "uncutgems": true,
}

// LeagueSlug converts "Forbidden Rites" into "forbiddenrites".
func LeagueSlug(league string) string {
	return strings.ToLower(strings.ReplaceAll(league, " ", ""))
}

// FetchScout returns bulk-tradable items (currency, runes, omens, ...) and
// the Divine Orb price in Exalted Orbs.
func FetchScout(ctx context.Context, c *http.Client, league string) ([]prices.CurrencyPrice, float64, error) {
	var items []scoutItem
	if err := getJSON(ctx, c, fmt.Sprintf(scoutAPIURL, LeagueSlug(league)), &items); err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return nil, 0, fmt.Errorf("poe2scout returned an empty list")
	}

	var out []prices.CurrencyPrice
	divineEx := 0.0
	for _, it := range items {
		if it.ApiId == "divine" {
			divineEx = it.CurrentPrice
		}
		cat := strings.ToLower(it.CategoryApiId)
		if cat == "" {
			cat = "currency"
		}
		// Items with a Name/Type are uniques; they are priced by poe.ninja.
		isUnique := (it.Name != nil && *it.Name != "") || (it.Type != nil && *it.Type != "")
		if scoutSkipCategories[cat] || isUnique || it.Text == "" || it.CurrentPrice <= 0 {
			continue
		}
		out = append(out, prices.CurrencyPrice{
			Name:     strings.TrimSpace(it.Text),
			APIID:    it.ApiId,
			Category: cat,
			ValueEx:  it.CurrentPrice,
		})
	}
	return out, divineEx, nil
}
