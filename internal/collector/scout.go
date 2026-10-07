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
// poe.ninja, waystones have dedicated tier rules.
var scoutSkipCategories = map[string]bool{
	"accessory": true, "armour": true, "weapon": true, "flask": true,
	"jewel": true, "sanctum": true, "talismans": true, "map": true,
	"waystones": true,
}

// scoutUncutGem tells the uncut gems priced per level from the rest of
// their category: skill and spirit gems ("Uncut Spirit Gem (Level 20)"),
// whose worth runs anything but in step with the level (20 dear, 18 and 19
// cheap, 1 and 2 dear again, as they drop only while levelling). Support
// gems stay with their level slider.
func scoutUncutGem(text string) bool {
	return strings.HasPrefix(text, "Uncut Skill Gem (Level ") || strings.HasPrefix(text, "Uncut Spirit Gem (Level ")
}

// LeagueSlug converts "Forbidden Rites" into "forbiddenrites".
func LeagueSlug(league string) string {
	return strings.ToLower(strings.ReplaceAll(league, " ", ""))
}

// FetchScout returns bulk-tradable items (currency, runes, omens, ...) and
// the Divine Orb price in Exalted Orbs.
func FetchScout(ctx context.Context, c *http.Client, league string) ([]prices.CurrencyPrice, float64, error) {
	items, err := fetchScoutItems(ctx, c, league)
	if err != nil {
		return nil, 0, err
	}
	out, divineEx := scoutCurrency(items)
	return out, divineEx, nil
}

// scoutCurrency keeps the bulk-tradable items of poe2scout's list and reads
// the Divine Orb's price off it.
func scoutCurrency(items []scoutItem) ([]prices.CurrencyPrice, float64) {
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
		if cat == "uncutgems" && !scoutUncutGem(strings.TrimSpace(it.Text)) {
			continue
		}
		out = append(out, prices.CurrencyPrice{
			Name:     strings.TrimSpace(it.Text),
			APIID:    it.ApiId,
			Category: cat,
			ValueEx:  it.CurrentPrice,
		})
	}
	return out, divineEx
}

func fetchScoutItems(ctx context.Context, c *http.Client, league string) ([]scoutItem, error) {
	var items []scoutItem
	if err := getJSON(ctx, c, fmt.Sprintf(scoutAPIURL, LeagueSlug(league)), &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("poe2scout returned an empty list")
	}
	return items, nil
}

// FetchScoutUncutGems returns poe2scout's uncut gem prices by name ("Uncut
// Spirit Gem (Level 20)"), in Exalted Orbs. The shared price list leaves
// them out (the loot filter shows uncut gems by level, not by price); the
// Expedition labels ask for them. Their values follow the in-game Currency
// Exchange (checked 2026-10-07: Spirit 20 5384 ex against 75 Chaos in game),
// unlike the trade site's bulk listings, which scatter from 1 to 30 Divine.
func FetchScoutUncutGems(ctx context.Context, c *http.Client, league string) (map[string]float64, error) {
	items, err := fetchScoutItems(ctx, c, league)
	if err != nil {
		return nil, err
	}
	return uncutGemPrices(items), nil
}

func uncutGemPrices(items []scoutItem) map[string]float64 {
	out := map[string]float64{}
	for _, it := range items {
		if strings.EqualFold(it.CategoryApiId, "uncutgems") && it.Text != "" && it.CurrentPrice > 0 {
			out[strings.TrimSpace(it.Text)] = it.CurrentPrice
		}
	}
	return out
}
