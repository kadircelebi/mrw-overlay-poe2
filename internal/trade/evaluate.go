package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type SelectedStat struct {
	ID       string   `json:"id"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Weight   *float64 `json:"weight,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
}

type SelectedFilter struct {
	Group  string   `json:"group"`
	ID     string   `json:"id"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Option string   `json:"option,omitempty"`
}

type SelectedStatGroup struct {
	Type  string         `json:"type"`
	Min   *float64       `json:"min,omitempty"`
	Stats []SelectedStat `json:"stats"`
}

type EvaluateRequest struct {
	League   string              `json:"league"`
	Name     string              `json:"name"`
	BaseType string              `json:"baseType"`
	Rarity   string              `json:"rarity"`
	Status   string              `json:"status"`
	Stats    []SelectedStat      `json:"stats"`
	Groups   []SelectedStatGroup `json:"groups"`
	Filters  []SelectedFilter    `json:"filters"`
}

type EvaluatedProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EvaluatedMod struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Name        string `json:"name,omitempty"`
	Tier        string `json:"tier,omitempty"`
}

type EvaluatedItem struct {
	Name         string `json:"name"`
	BaseType     string `json:"baseType"`
	Rarity       string `json:"rarity"`
	ItemLevel    int    `json:"itemLevel"`
	Icon         string `json:"icon"`
	Unidentified bool   `json:"unidentified"`
	Fractured    bool   `json:"fractured"`
	Corrupted    bool   `json:"corrupted"`
	Sanctified   bool   `json:"sanctified"`
	// DPS figures are computed from the listing's weapon properties, the same
	// way the trade site shows them; zero for non-weapons.
	DPS          float64             `json:"dps"`
	PhysicalDPS  float64             `json:"physicalDps"`
	ElementalDPS float64             `json:"elementalDps"`
	Properties   []EvaluatedProperty `json:"properties"`
	Mods         []EvaluatedMod      `json:"mods"`
}

type EvaluatedListing struct {
	ID           string        `json:"id"`
	Amount       float64       `json:"amount"`
	Currency     string        `json:"currency"`
	Account      string        `json:"account"`
	HideoutToken string        `json:"hideoutToken,omitempty"`
	Listed       string        `json:"listed"`
	Item         EvaluatedItem `json:"item"`
}

type Evaluation struct {
	SearchID string             `json:"searchId"`
	TradeURL string             `json:"tradeUrl"`
	Total    int                `json:"total"`
	Listings []EvaluatedListing `json:"listings"`
}

type queryValue struct {
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Weight *float64 `json:"weight,omitempty"`
	Option string   `json:"option,omitempty"`
}

type queryStat struct {
	ID       string      `json:"id"`
	Value    *queryValue `json:"value,omitempty"`
	Disabled bool        `json:"disabled,omitempty"`
}

type queryStatGroup struct {
	Type    string      `json:"type"`
	Filters []queryStat `json:"filters"`
	Value   *queryValue `json:"value,omitempty"`
}

type evaluateSearchResponse struct {
	ID     string   `json:"id"`
	Result []string `json:"result"`
	Total  int      `json:"total"`
}

type evaluatedFetchResponse struct {
	Result []struct {
		ID      string `json:"id"`
		Listing struct {
			Indexed      string `json:"indexed"`
			HideoutToken string `json:"hideout_token"`
			Account      struct {
				Name              string `json:"name"`
				LastCharacterName string `json:"lastCharacterName"`
			} `json:"account"`
			Price *struct {
				Amount   float64 `json:"amount"`
				Currency string  `json:"currency"`
			} `json:"price"`
		} `json:"listing"`
		Item struct {
			Name       string `json:"name"`
			TypeLine   string `json:"typeLine"`
			BaseType   string `json:"baseType"`
			Rarity     string `json:"rarity"`
			Ilvl       int    `json:"ilvl"`
			Icon       string `json:"icon"`
			Identified bool   `json:"identified"`
			Fractured  bool   `json:"fractured"`
			Corrupted  bool   `json:"corrupted"`
			Sanctified bool   `json:"sanctified"`
			Properties []struct {
				Name   string          `json:"name"`
				Values [][]interface{} `json:"values"`
			} `json:"properties"`
			ImplicitMods   []evaluatedModLine `json:"implicitMods"`
			ExplicitMods   []evaluatedModLine `json:"explicitMods"`
			CraftedMods    []evaluatedModLine `json:"craftedMods"`
			DesecratedMods []evaluatedModLine `json:"desecratedMods"`
			FracturedMods  []evaluatedModLine `json:"fracturedMods"`
			RuneMods       []evaluatedModLine `json:"runeMods"`
			EnchantMods    []evaluatedModLine `json:"enchantMods"`
		} `json:"item"`
	} `json:"result"`
}

type evaluatedModLine struct {
	Description string `json:"description"`
	Mods        []struct {
		Name string `json:"name"`
		Tier string `json:"tier"`
	} `json:"mods"`
}

// Evaluate runs one user-requested price search and fetches the first ten
// listings. Search and fetch use the same conservative limiters as the scanner.
func (c *Client) Evaluate(ctx context.Context, in EvaluateRequest) (Evaluation, error) {
	if strings.TrimSpace(in.BaseType) == "" && strings.TrimSpace(in.Name) == "" && !hasCategory(in.Filters) {
		return Evaluation{}, fmt.Errorf("search needs a base type, a unique name or an item category")
	}
	league := strings.TrimSpace(in.League)
	if league == "" {
		league = c.league
	}
	status := in.Status
	if status == "" {
		status = "securable"
	}

	convertStats := func(stats []SelectedStat) []queryStat {
		statFilters := make([]queryStat, 0, len(stats))
		for _, st := range stats {
			if st.ID == "" || st.Disabled {
				continue
			}
			q := queryStat{ID: st.ID}
			if st.Min != nil || st.Max != nil || st.Weight != nil {
				q.Value = &queryValue{Min: st.Min, Max: st.Max, Weight: st.Weight}
			}
			statFilters = append(statFilters, q)
		}
		return statFilters
	}
	statGroups := make([]queryStatGroup, 0, max(1, len(in.Groups)))
	for _, group := range in.Groups {
		filters := convertStats(group.Stats)
		if len(filters) == 0 {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(group.Type))
		switch kind {
		case "and", "not", "if", "count", "weight", "weight2", "skill":
		default:
			kind = "and"
		}
		out := queryStatGroup{Type: kind, Filters: filters}
		if group.Min != nil {
			out.Value = &queryValue{Min: group.Min}
		}
		statGroups = append(statGroups, out)
	}
	if len(statGroups) == 0 {
		statGroups = append(statGroups, queryStatGroup{Type: "and", Filters: convertStats(in.Stats)})
	}

	query := map[string]interface{}{
		"status": map[string]string{"option": status},
		"stats":  statGroups,
	}
	if in.BaseType != "" {
		query["type"] = in.BaseType
	}
	// The name is only sent for uniques by the overlay; keep it even when the
	// user widens the rarity (e.g. to include foil versions).
	if in.Name != "" {
		query["name"] = in.Name
	}

	groups := map[string]map[string]interface{}{}
	if in.Rarity != "" {
		groups["type_filters"] = map[string]interface{}{
			"rarity": queryValue{Option: strings.ToLower(in.Rarity)},
		}
	}
	for _, f := range in.Filters {
		if f.Group == "" || f.ID == "" || (f.Min == nil && f.Max == nil && f.Option == "") {
			continue
		}
		g := groups[f.Group]
		if g == nil {
			g = map[string]interface{}{}
			groups[f.Group] = g
		}
		g[f.ID] = queryValue{Min: f.Min, Max: f.Max, Option: f.Option}
	}
	if len(groups) > 0 {
		wrapped := map[string]interface{}{}
		for id, filters := range groups {
			wrapped[id] = map[string]interface{}{"filters": filters}
		}
		query["filters"] = wrapped
	}

	body, err := json.Marshal(map[string]interface{}{
		"query": query,
		"sort":  map[string]string{"price": "asc"},
	})
	if err != nil {
		return Evaluation{}, err
	}
	searchURL := fmt.Sprintf("%s/search/poe2/%s", apiBase, url.PathEscape(league))
	req, err := http.NewRequest(http.MethodPost, searchURL, bytes.NewReader(body))
	if err != nil {
		return Evaluation{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var search evaluateSearchResponse
	if err := c.do(ctx, c.Search, req, &search); err != nil {
		return Evaluation{}, err
	}
	out := Evaluation{
		SearchID: search.ID,
		TradeURL: fmt.Sprintf("https://www.pathofexile.com/trade2/search/poe2/%s/%s", url.PathEscape(league), search.ID),
		Total:    search.Total,
		Listings: []EvaluatedListing{},
	}
	if len(search.Result) == 0 {
		return out, nil
	}
	ids := search.Result
	if len(ids) > 10 {
		ids = ids[:10]
	}
	fetchURL := fmt.Sprintf("%s/fetch/%s?query=%s&realm=poe2", apiBase, strings.Join(ids, ","), url.QueryEscape(search.ID))
	fetchReq, err := http.NewRequest(http.MethodGet, fetchURL, nil)
	if err != nil {
		return Evaluation{}, err
	}
	var fetched evaluatedFetchResponse
	if err := c.do(ctx, c.Fetch, fetchReq, &fetched); err != nil {
		return Evaluation{}, err
	}
	for _, row := range fetched.Result {
		if row.Listing.Price == nil {
			continue
		}
		account := row.Listing.Account.Name
		if account == "" {
			account = row.Listing.Account.LastCharacterName
		}
		entry := EvaluatedListing{
			ID: row.ID, Amount: row.Listing.Price.Amount, Currency: row.Listing.Price.Currency,
			Account: account, HideoutToken: row.Listing.HideoutToken, Listed: row.Listing.Indexed,
			Item: EvaluatedItem{Name: row.Item.Name, BaseType: row.Item.BaseType, Rarity: row.Item.Rarity, ItemLevel: row.Item.Ilvl, Icon: row.Item.Icon, Unidentified: !row.Item.Identified, Fractured: row.Item.Fractured, Corrupted: row.Item.Corrupted, Sanctified: row.Item.Sanctified, Properties: []EvaluatedProperty{}, Mods: []EvaluatedMod{}},
		}
		if entry.Item.BaseType == "" {
			entry.Item.BaseType = row.Item.TypeLine
		}
		for _, p := range row.Item.Properties {
			name, value := formatTradeProperty(p.Name, p.Values)
			entry.Item.Properties = append(entry.Item.Properties, EvaluatedProperty{Name: name, Value: value})
		}
		addWeaponDPS(&entry.Item)
		appendMods := func(kind string, lines []evaluatedModLine) {
			for _, line := range lines {
				mod := EvaluatedMod{Type: kind, Description: cleanTradeDescription(line.Description)}
				if len(line.Mods) > 0 {
					mod.Name, mod.Tier = line.Mods[0].Name, line.Mods[0].Tier
				}
				entry.Item.Mods = append(entry.Item.Mods, mod)
			}
		}
		appendMods("implicit", row.Item.ImplicitMods)
		appendMods("explicit", row.Item.ExplicitMods)
		appendMods("fractured", row.Item.FracturedMods)
		appendMods("desecrated", row.Item.DesecratedMods)
		appendMods("crafted", row.Item.CraftedMods)
		appendMods("rune", row.Item.RuneMods)
		appendMods("enchant", row.Item.EnchantMods)
		out.Listings = append(out.Listings, entry)
	}
	return out, nil
}

var damageRangeRE = regexp.MustCompile(`(\d+(?:\.\d+)?)-(\d+(?:\.\d+)?)`)

// addWeaponDPS derives physical, elemental and total DPS from the listing's
// damage and attack speed properties and appends them for display.
func addWeaponDPS(item *EvaluatedItem) {
	var physical, elemental, chaos, aps float64
	for _, p := range item.Properties {
		avg := 0.0
		for _, m := range damageRangeRE.FindAllStringSubmatch(p.Value, -1) {
			lo, _ := strconv.ParseFloat(m[1], 64)
			hi, _ := strconv.ParseFloat(m[2], 64)
			avg += (lo + hi) / 2
		}
		switch p.Name {
		case "Physical Damage":
			physical += avg
		case "Elemental Damage", "Fire Damage", "Cold Damage", "Lightning Damage":
			elemental += avg
		case "Chaos Damage":
			chaos += avg
		case "Attacks per Second":
			aps, _ = strconv.ParseFloat(strings.TrimSpace(p.Value), 64)
		}
	}
	if aps <= 0 || physical+elemental+chaos <= 0 {
		return
	}
	round := func(v float64) float64 { return math.Round(v*100) / 100 }
	item.PhysicalDPS, item.ElementalDPS = round(physical*aps), round(elemental*aps)
	item.DPS = round((physical + elemental + chaos) * aps)
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	if item.PhysicalDPS > 0 {
		item.Properties = append(item.Properties, EvaluatedProperty{Name: "Physical DPS", Value: format(item.PhysicalDPS)})
	}
	if item.ElementalDPS > 0 {
		item.Properties = append(item.Properties, EvaluatedProperty{Name: "Elemental DPS", Value: format(item.ElementalDPS)})
	}
	item.Properties = append(item.Properties, EvaluatedProperty{Name: "DPS", Value: format(item.DPS)})
}

// formatTradeProperty turns an API property into readable text. Some names
// are templates ("Recovers {0} Life over {1} Seconds") whose placeholders take
// the values in order; the rest are "Name: value[, value…]".
func formatTradeProperty(name string, values [][]interface{}) (string, string) {
	name = cleanTradeDescription(name)
	texts := make([]string, 0, len(values))
	for _, v := range values {
		if len(v) > 0 {
			texts = append(texts, fmt.Sprint(v[0]))
		}
	}
	if strings.Contains(name, "{0}") {
		for i, text := range texts {
			name = strings.ReplaceAll(name, "{"+strconv.Itoa(i)+"}", text)
		}
		return name, ""
	}
	return name, strings.Join(texts, ", ")
}

func cleanTradeDescription(s string) string {
	// The API embeds game glossary links as [Key|visible text]. The overlay
	// needs only the same readable description the player sees in game.
	for {
		start := strings.IndexByte(s, '[')
		if start < 0 {
			break
		}
		endRel := strings.IndexByte(s[start:], ']')
		if endRel < 0 {
			break
		}
		end := start + endRel
		inside := s[start+1 : end]
		if pipe := strings.IndexByte(inside, '|'); pipe >= 0 {
			inside = inside[pipe+1:]
		}
		s = s[:start] + inside + s[end+1:]
	}
	return s
}

func hasCategory(filters []SelectedFilter) bool {
	for _, f := range filters {
		if f.Group == "type_filters" && f.ID == "category" && f.Option != "" {
			return true
		}
	}
	return false
}
