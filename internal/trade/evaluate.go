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
	// Input is a free-text filter such as the seller account name.
	Input string `json:"input,omitempty"`
}

type SelectedStatGroup struct {
	Type  string         `json:"type"`
	Min   *float64       `json:"min,omitempty"`
	Max   *float64       `json:"max,omitempty"`
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
	// Sort is a trade sort key ("price", "dps", "stat.explicit.stat_…");
	// empty means price. SortDir is "asc" or "desc".
	Sort    string `json:"sort,omitempty"`
	SortDir string `json:"sortDir,omitempty"`
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
	// StatID is the trade stat of this line, so the result list can sort by
	// any affix it shows. Empty when the line matched no stat for certain.
	StatID string `json:"statId,omitempty"`
	// Parts splits a line that sums several affixes, so each affix shows its
	// own share. GGG sends only the sum and each affix's roll range, so a
	// share is the narrowest range the sum allows, exact when it is pinned.
	Parts []EvaluatedModPart `json:"parts,omitempty"`
}

type EvaluatedModPart struct {
	Name        string `json:"name,omitempty"`
	Tier        string `json:"tier,omitempty"`
	Description string `json:"description"`
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
	// TwiceCorrupted is the trade site's doubleCorrupted flag.
	TwiceCorrupted bool `json:"twiceCorrupted"`
	// Mirrored is the trade site's duplicated flag (a Mirror of Kalandra copy).
	Mirrored   bool `json:"mirrored"`
	Sanctified bool `json:"sanctified"`
	// Sockets counts the augmentable (rune) sockets, filled or empty.
	Sockets int `json:"sockets"`
	// DPS figures are computed from the listing's weapon properties, the same
	// way the trade site shows them; zero for non-weapons.
	DPS          float64             `json:"dps"`
	PhysicalDPS  float64             `json:"physicalDps"`
	ElementalDPS float64             `json:"elementalDps"`
	Properties   []EvaluatedProperty `json:"properties"`
	Mods         []EvaluatedMod      `json:"mods"`
	// StatHashes are the stat numbers ("stat_1050105434") GGG reports for the
	// item. They do not line up with the mod lines, but they settle which of
	// two stats with the same wording (local or global) a line is.
	StatHashes []string `json:"-"`
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
	SearchID string `json:"searchId"`
	TradeURL string `json:"tradeUrl"`
	Total    int    `json:"total"`
	// ResultIDs are every listing the search returned (GGG caps it at 100);
	// Listings holds the first page, the rest are fetched as the list scrolls.
	ResultIDs []string           `json:"resultIds"`
	Listings  []EvaluatedListing `json:"listings"`
	// SignedIn reports that the search went out with a pathofexile.com
	// session; only then can a listing's hideout travel be used.
	SignedIn bool `json:"signedIn"`
}

type queryValue struct {
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Weight *float64 `json:"weight,omitempty"`
	Option string   `json:"option,omitempty"`
	Input  string   `json:"input,omitempty"`
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
			Name            string            `json:"name"`
			TypeLine        string            `json:"typeLine"`
			BaseType        string            `json:"baseType"`
			Rarity          string            `json:"rarity"`
			Ilvl            int               `json:"ilvl"`
			Icon            string            `json:"icon"`
			Identified      bool              `json:"identified"`
			Fractured       bool              `json:"fractured"`
			Corrupted       bool              `json:"corrupted"`
			DoubleCorrupted bool              `json:"doubleCorrupted"`
			Duplicated      bool              `json:"duplicated"`
			Sanctified      bool              `json:"sanctified"`
			Sockets         []json.RawMessage `json:"sockets"`
			Properties      []struct {
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
			Extended       struct {
				Hashes map[string][][]json.RawMessage `json:"hashes"`
			} `json:"extended"`
		} `json:"item"`
	} `json:"result"`
}

type evaluatedModLine struct {
	Description string `json:"description"`
	// Hash is the line's trade stat ("stat.explicit.stat_1999113824").
	Hash string `json:"hash"`
	// Mods are the affixes behind the line: the trade site shows one line per
	// stat, so two affixes of the same stat arrive summed with both listed.
	Mods []struct {
		Name       string `json:"name"`
		Tier       string `json:"tier"`
		Magnitudes []struct {
			Min json.Number `json:"min"`
			Max json.Number `json:"max"`
		} `json:"magnitudes"`
	} `json:"mods"`
}

// search runs a request's query and returns GGG's search id and first result
// ids, and the league it ran in. It spends one search of the quota.
func (c *Client) search(ctx context.Context, in EvaluateRequest) (evaluateSearchResponse, string, error) {
	query, err := buildEvaluateQuery(in)
	if err != nil {
		return evaluateSearchResponse{}, "", err
	}
	league := strings.TrimSpace(in.League)
	if league == "" {
		league = c.league
	}
	sortKey, sortDir, err := evaluateSort(in)
	if err != nil {
		return evaluateSearchResponse{}, "", err
	}
	body, err := json.Marshal(map[string]interface{}{
		"query": query,
		"sort":  map[string]string{sortKey: sortDir},
	})
	if err != nil {
		return evaluateSearchResponse{}, "", err
	}
	searchURL := fmt.Sprintf("%s/search/poe2/%s", apiBase, url.PathEscape(league))
	req, err := http.NewRequest(http.MethodPost, searchURL, bytes.NewReader(body))
	if err != nil {
		return evaluateSearchResponse{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var search evaluateSearchResponse
	if err := c.do(ctx, c.Search, req, &search); err != nil {
		return evaluateSearchResponse{}, "", err
	}
	return search, league, nil
}

// Evaluate runs one user-requested price search and fetches the first ten
// listings. Search and fetch use the same conservative limiters as the scanner.
func (c *Client) Evaluate(ctx context.Context, in EvaluateRequest) (Evaluation, error) {
	search, league, err := c.search(ctx, in)
	if err != nil {
		return Evaluation{}, err
	}
	out := Evaluation{
		SearchID:  search.ID,
		TradeURL:  fmt.Sprintf("https://www.pathofexile.com/trade2/search/poe2/%s/%s", url.PathEscape(league), search.ID),
		Total:     search.Total,
		ResultIDs: search.Result,
		Listings:  []EvaluatedListing{},
	}
	if out.ResultIDs == nil {
		out.ResultIDs = []string{}
	}
	if len(search.Result) == 0 {
		return out, nil
	}
	ids := search.Result
	if len(ids) > FetchPageSize {
		ids = ids[:FetchPageSize]
	}
	out.Listings, err = c.FetchEvaluated(ctx, search.ID, ids)
	if err != nil {
		return Evaluation{}, err
	}
	return out, nil
}

// FetchPageSize is how many listings GGG returns per fetch call.
const FetchPageSize = 10

// FetchEvaluated loads one page of an earlier search's listings. It spends
// fetch quota only, so scrolling further down a result list never costs a
// search.
func (c *Client) FetchEvaluated(ctx context.Context, searchID string, ids []string) ([]EvaluatedListing, error) {
	if searchID == "" || len(ids) == 0 {
		return []EvaluatedListing{}, nil
	}
	if len(ids) > FetchPageSize {
		return nil, fmt.Errorf("fetch takes at most %d ids", FetchPageSize)
	}
	for _, id := range ids {
		if !listingIDRE.MatchString(id) {
			return nil, fmt.Errorf("invalid listing id %q", id)
		}
	}
	fetchURL := fmt.Sprintf("%s/fetch/%s?query=%s&realm=poe2", apiBase, strings.Join(ids, ","), url.QueryEscape(searchID))
	fetchReq, err := http.NewRequest(http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, err
	}
	var fetched evaluatedFetchResponse
	if err := c.do(ctx, c.Fetch, fetchReq, &fetched); err != nil {
		return nil, err
	}
	return evaluatedListings(fetched), nil
}

// FetchLiveToken loads the listings a live search pushed. GGG sends a token
// ({"result": "<token>", "count": N}) instead of listing ids; the trade site
// fetches /fetch/<token> for them, with no query id.
func (c *Client) FetchLiveToken(ctx context.Context, token string) ([]EvaluatedListing, error) {
	if len(token) < 8 || len(token) > 8192 || !liveTokenRE.MatchString(token) {
		return nil, fmt.Errorf("invalid live search token")
	}
	req, err := http.NewRequest(http.MethodGet, apiBase+"/fetch/"+token, nil)
	if err != nil {
		return nil, err
	}
	var fetched evaluatedFetchResponse
	if err := c.do(ctx, c.Fetch, req, &fetched); err != nil {
		return nil, err
	}
	return evaluatedListings(fetched), nil
}

func evaluatedListings(fetched evaluatedFetchResponse) []EvaluatedListing {
	listings := []EvaluatedListing{}
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
			Item: EvaluatedItem{Name: row.Item.Name, BaseType: row.Item.BaseType, Rarity: row.Item.Rarity, ItemLevel: row.Item.Ilvl, Icon: row.Item.Icon, Unidentified: !row.Item.Identified, Fractured: row.Item.Fractured, Corrupted: row.Item.Corrupted, TwiceCorrupted: row.Item.DoubleCorrupted, Mirrored: row.Item.Duplicated, Sanctified: row.Item.Sanctified, Sockets: len(row.Item.Sockets), Properties: []EvaluatedProperty{}, Mods: []EvaluatedMod{}},
		}
		if entry.Item.BaseType == "" {
			entry.Item.BaseType = row.Item.TypeLine
		}
		// A magic item's name is its whole type line ("Athlete's Sirenscale
		// Gloves of Archaeology"), as the game shows it.
		if strings.EqualFold(row.Item.Rarity, "magic") && row.Item.Name == "" && row.Item.TypeLine != entry.Item.BaseType {
			entry.Item.Name = row.Item.TypeLine
		}
		for _, p := range row.Item.Properties {
			name, value := formatTradeProperty(p.Name, p.Values)
			entry.Item.Properties = append(entry.Item.Properties, EvaluatedProperty{Name: name, Value: value})
		}
		addWeaponDPS(&entry.Item)
		for _, hashes := range row.Item.Extended.Hashes {
			for _, pair := range hashes {
				var id string
				if len(pair) > 0 && json.Unmarshal(pair[0], &id) == nil {
					if dot := strings.IndexByte(id, '.'); dot >= 0 {
						id = id[dot+1:]
					}
					entry.Item.StatHashes = append(entry.Item.StatHashes, id)
				}
			}
		}
		appendMods := func(kind string, lines []evaluatedModLine) {
			for _, line := range lines {
				mod := EvaluatedMod{Type: kind, Description: cleanTradeDescription(line.Description), StatID: strings.TrimPrefix(line.Hash, "stat.")}
				if len(line.Mods) > 0 {
					mod.Name, mod.Tier = line.Mods[0].Name, line.Mods[0].Tier
				}
				mod.Parts = splitModLine(mod.Description, line)
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
		listings = append(listings, entry)
	}
	return listings
}

var (
	listingIDRE = regexp.MustCompile(`^[0-9a-f]{16,128}$`)
	liveTokenRE = regexp.MustCompile(`^[A-Za-z0-9._\-]+$`)
	// sortKeyRE admits the trade site's sort keys: plain names ("price",
	// "pdps") and stat paths ("stat.explicit.stat_1509134228",
	// "stat.pseudo.pseudo_total_life"). GGG rejects unknown keys itself.
	sortKeyRE = regexp.MustCompile(`^(?:[a-z_]+|statgroup\.[0-9]+|stat\.[a-z]+\.[a-z0-9_]+(?:\|[0-9]+)?)$`)
)

// evaluateSort returns the request's sort, defaulting to the cheapest first.
// A stat sort only makes sense for a stat the query filters on; the trade
// site behaves the same way.
func evaluateSort(in EvaluateRequest) (string, string, error) {
	key := strings.TrimSpace(in.Sort)
	if key == "" {
		key = "price"
	}
	if !sortKeyRE.MatchString(key) {
		return "", "", fmt.Errorf("invalid sort key %q", key)
	}
	dir := strings.ToLower(strings.TrimSpace(in.SortDir))
	switch dir {
	case "asc", "desc":
	case "":
		dir = "desc"
		if key == "price" {
			dir = "asc"
		}
	default:
		return "", "", fmt.Errorf("invalid sort direction %q", in.SortDir)
	}
	return key, dir, nil
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

// buildEvaluateQuery turns every filled field of the request into the trade
// query, so a Market search from scratch sends exactly what the user entered.
func buildEvaluateQuery(in EvaluateRequest) (map[string]interface{}, error) {
	if !hasCriterion(in) {
		return nil, fmt.Errorf("search needs an item, a stat or a filter")
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
		if group.Min != nil || group.Max != nil {
			out.Value = &queryValue{Min: group.Min, Max: group.Max}
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
	if rarity := strings.ToLower(in.Rarity); tradeRarities[rarity] {
		groups["type_filters"] = map[string]interface{}{
			"rarity": queryValue{Option: rarity},
		}
	}
	for _, f := range in.Filters {
		input := strings.TrimSpace(f.Input)
		if f.Group == "" || f.ID == "" || (f.Min == nil && f.Max == nil && f.Option == "" && input == "") {
			continue
		}
		if f.Group == "type_filters" && f.ID == "rarity" && !tradeRarities[strings.ToLower(f.Option)] {
			continue
		}
		g := groups[f.Group]
		if g == nil {
			g = map[string]interface{}{}
			groups[f.Group] = g
		}
		g[f.ID] = queryValue{Min: f.Min, Max: f.Max, Option: f.Option, Input: input}
	}
	if len(groups) > 0 {
		wrapped := map[string]interface{}{}
		for id, filters := range groups {
			wrapped[id] = map[string]interface{}{"filters": filters}
		}
		query["filters"] = wrapped
	}

	return query, nil
}

// tradeRarities are the rarity options the trade site accepts. The game also
// prints "Currency", "Gem" and the like, which the API rejects outright
// ("Unknown rarity type"), so those searches go without a rarity.
var tradeRarities = map[string]bool{"normal": true, "magic": true, "rare": true, "unique": true, "uniquefoil": true, "nonunique": true}

// hasCriterion reports whether the request narrows the search at all; the
// rarity alone does not, since an empty search would spend quota on nothing.
func hasCriterion(in EvaluateRequest) bool {
	if strings.TrimSpace(in.BaseType) != "" || strings.TrimSpace(in.Name) != "" {
		return true
	}
	for _, st := range in.Stats {
		if st.ID != "" && !st.Disabled {
			return true
		}
	}
	for _, group := range in.Groups {
		for _, st := range group.Stats {
			if st.ID != "" && !st.Disabled {
				return true
			}
		}
	}
	for _, f := range in.Filters {
		if f.Group == "type_filters" && f.ID == "rarity" {
			continue
		}
		if f.Min != nil || f.Max != nil || f.Option != "" || strings.TrimSpace(f.Input) != "" {
			return true
		}
	}
	return false
}

// splitModLine divides a summed stat line among the affixes behind it. Each
// affix's share is bounded by its own roll range and by what the others can
// add up to; lines whose numbers do not line up with the ranges stay whole.
func splitModLine(description string, line evaluatedModLine) []EvaluatedModPart {
	if len(line.Mods) < 2 {
		return nil
	}
	totals := numbersIn(description)
	type span struct{ lo, hi float64 }
	ranges := make([][]span, len(line.Mods))
	for i, mod := range line.Mods {
		if len(mod.Magnitudes) != len(totals) || len(totals) == 0 {
			return nil
		}
		for _, m := range mod.Magnitudes {
			lo, err1 := m.Min.Float64()
			hi, err2 := m.Max.Float64()
			if err1 != nil || err2 != nil {
				return nil
			}
			if lo > hi {
				lo, hi = hi, lo
			}
			ranges[i] = append(ranges[i], span{lo, hi})
		}
	}
	parts := make([]EvaluatedModPart, len(line.Mods))
	for i, mod := range line.Mods {
		shares := make([]string, len(totals))
		for v, total := range totals {
			othersLo, othersHi := 0.0, 0.0
			for j := range ranges {
				if j != i {
					othersLo += ranges[j][v].lo
					othersHi += ranges[j][v].hi
				}
			}
			lo := math.Max(ranges[i][v].lo, total-othersHi)
			hi := math.Min(ranges[i][v].hi, total-othersLo)
			if lo > hi {
				// The ranges cannot make this sum (rounding, a changed mod):
				// show the roll range rather than an impossible share.
				lo, hi = ranges[i][v].lo, ranges[i][v].hi
			}
			shares[v] = formatShare(lo, hi)
		}
		parts[i] = EvaluatedModPart{Name: mod.Name, Tier: mod.Tier, Description: replaceNumbers(description, shares)}
	}
	return parts
}

var lineNumberRE = regexp.MustCompile(`\d+(?:\.\d+)?`)

func numbersIn(s string) []float64 {
	var out []float64
	for _, m := range lineNumberRE.FindAllString(s, -1) {
		v, _ := strconv.ParseFloat(m, 64)
		out = append(out, v)
	}
	return out
}

func replaceNumbers(s string, values []string) string {
	i := 0
	return lineNumberRE.ReplaceAllStringFunc(s, func(m string) string {
		if i >= len(values) {
			return m
		}
		i++
		return values[i-1]
	})
}

func formatShare(lo, hi float64) string {
	format := func(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }
	if hi-lo < 0.005 {
		return format(lo)
	}
	return format(lo) + "–" + format(hi)
}
