package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"poe2filter/internal/prices"
)

const ninjaOverviewURL = "https://poe.ninja/poe2/api/economy/stash/current/item/overview"

// NinjaUniqueTypes are the poe.ninja overview types that contain uniques.
var NinjaUniqueTypes = []string{
	"UniqueArmours",
	"UniqueWeapons",
	"UniqueAccessories",
	"UniqueFlasks",
	"UniqueCharms",
	"UniqueJewels",
	"UniqueSanctumRelics",
	"UniqueTablets",
}

type ninjaResponse struct {
	Core struct {
		Rates map[string]float64 `json:"rates"` // units per 1 Divine Orb
	} `json:"core"`
	Lines []struct {
		Name         string  `json:"name"`
		BaseType     string  `json:"baseType"`
		PrimaryValue float64 `json:"primaryValue"` // in Divine Orbs
		ListingCount int     `json:"listingCount"`
	} `json:"lines"`
}

// NinjaResult holds the uniques fetched from poe.ninja.
type NinjaResult struct {
	// Uniques lists every unique keyed by its ground base type. Values are
	// in Divine Orbs; ToExalted converts them once the final rate is known.
	Uniques map[string][]NinjaUnique
	// DivineEx and ChaosEx are exchange rates (0 when not reported).
	DivineEx float64
	ChaosEx  float64
	// Failed lists overview types that could not be fetched.
	Failed map[string]error
}

// NinjaUnique is a unique price as reported by poe.ninja (in Divine Orbs).
type NinjaUnique struct {
	Name     string
	Category string
	ValueDiv float64
	Listings int
}

// GroundBase maps a poe.ninja base type to the base type that actually drops.
// Identified uniques on "Runeforged"/"Runemastered" bases drop (and are
// chanced) as the plain base, which is what the loot filter matches on.
func GroundBase(base string) string {
	b := strings.TrimSpace(strings.ReplaceAll(base, "\"", ""))
	b = strings.TrimPrefix(b, "Runeforged ")
	b = strings.TrimPrefix(b, "Runemastered ")
	return strings.TrimSpace(b)
}

// FetchNinjaUniques fetches all unique categories concurrently. Categories that
// fail are reported in Failed instead of silently disappearing.
func FetchNinjaUniques(ctx context.Context, c *http.Client, league string) NinjaResult {
	res := NinjaResult{
		Uniques: make(map[string][]NinjaUnique),
		Failed:  make(map[string]error),
	}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, typ := range NinjaUniqueTypes {
		wg.Add(1)
		go func(typ string) {
			defer wg.Done()
			u := fmt.Sprintf("%s?league=%s&type=%s", ninjaOverviewURL, url.QueryEscape(league), url.QueryEscape(typ))
			var data ninjaResponse
			err := getJSON(ctx, c, u, &data)

			mu.Lock()
			defer mu.Unlock()
			if err == nil && len(data.Lines) == 0 {
				err = fmt.Errorf("boş cevap")
			}
			if err != nil {
				res.Failed[typ] = err
				return
			}
			exPerDiv := data.Core.Rates["exalted"]
			chaosPerDiv := data.Core.Rates["chaos"]
			if exPerDiv > 0 {
				res.DivineEx = exPerDiv
				if chaosPerDiv > 0 {
					res.ChaosEx = exPerDiv / chaosPerDiv
				}
			}
			for _, line := range data.Lines {
				base := GroundBase(line.BaseType)
				if base == "" || line.Name == "" {
					continue
				}
				res.Uniques[base] = append(res.Uniques[base], NinjaUnique{
					Name:     line.Name,
					Category: typ,
					ValueDiv: line.PrimaryValue,
					Listings: line.ListingCount,
				})
			}
		}(typ)
	}
	wg.Wait()
	return res
}

// ToExalted converts poe.ninja uniques into snapshot uniques.
func (r NinjaResult) ToExalted(divineEx float64) map[string][]prices.Unique {
	out := make(map[string][]prices.Unique, len(r.Uniques))
	for base, list := range r.Uniques {
		for _, u := range list {
			out[base] = append(out[base], prices.Unique{
				Name:     u.Name,
				Category: u.Category,
				ValueEx:  u.ValueDiv * divineEx,
				Listings: u.Listings,
			})
		}
	}
	return out
}

// AggregateUniqueBases turns per-base unique lists into UniqueBase entries.
func AggregateUniqueBases(uniques map[string][]prices.Unique) map[string]prices.UniqueBase {
	out := make(map[string]prices.UniqueBase, len(uniques))
	for base, list := range uniques {
		var ub prices.UniqueBase
		for _, u := range list {
			if u.ValueEx >= ub.MaxEx {
				ub.MaxEx = u.ValueEx
				ub.TopName = u.Name
			}
		}
		ub.Uniques = list
		out[base] = ub
	}
	return out
}
