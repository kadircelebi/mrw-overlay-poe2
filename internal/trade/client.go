package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase   = "https://www.pathofexile.com/api/trade2"
	userAgent = "poe2-filter/0.2"
)

// Client is a rate-limited PoE2 trade API client.
type Client struct {
	http   *http.Client
	league string
	Search *Limiter
	Fetch  *Limiter
}

// NewClient creates a client that uses at most budget (0..1) of the IP quota.
func NewClient(league string, budget float64) *Client {
	return &Client{
		http:   &http.Client{Timeout: 30 * time.Second},
		league: league,
		Search: NewLimiter(budget, SearchSeedRules),
		Fetch:  NewLimiter(budget, FetchSeedRules),
	}
}

// NewInteractiveClient keeps the same rate-limit budgets as NewClient but
// allows the small bursts intended for user-initiated searches. It does not
// weaken the per-window or server-reported quota checks.
func NewInteractiveClient(league string, budget float64) *Client {
	client := NewClient(league, budget)
	client.Search.SetEvenPacing(false)
	client.Fetch.SetEvenPacing(false)
	return client
}

// Query is the JSON body of a trade search.
type Query struct {
	Query struct {
		Status struct {
			Option string `json:"option"`
		} `json:"status"`
		Type    string                 `json:"type,omitempty"`
		Stats   []statGroup            `json:"stats"`
		Filters map[string]filterGroup `json:"filters,omitempty"`
	} `json:"query"`
	Sort map[string]string `json:"sort"`
}

type statGroup struct {
	Type    string `json:"type"`
	Filters []any  `json:"filters"`
}

type filterGroup struct {
	Filters map[string]any `json:"filters"`
}

// Range is a min/max trade filter value.
type Range struct {
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// Option is a select-style trade filter value.
type Option struct {
	Option string `json:"option"`
}

// NewBaseQuery builds an instant-buyout search for one base type, cheapest first.
func NewBaseQuery(base, rarity string) *Query {
	q := &Query{Sort: map[string]string{"price": "asc"}}
	q.Query.Status.Option = "securable" // Instant Buyout only
	q.Query.Type = base
	q.Query.Stats = []statGroup{{Type: "and", Filters: []any{}}}
	q.Query.Filters = map[string]filterGroup{
		"type_filters": {Filters: map[string]any{"rarity": Option{Option: rarity}}},
	}
	return q
}

// SetMin sets a numeric minimum inside a filter group.
func (q *Query) SetMin(group, filter string, min int) {
	g, ok := q.Query.Filters[group]
	if !ok {
		g = filterGroup{Filters: map[string]any{}}
	}
	g.Filters[filter] = Range{Min: &min}
	q.Query.Filters[group] = g
}

// SearchResult is the response of a trade search.
type SearchResult struct {
	ID     string   `json:"id"`
	Result []string `json:"result"`
	Total  int      `json:"total"`
}

// Listing is the subset of a fetched listing we need.
type Listing struct {
	BaseType string
	TypeLine string
	Class    string
	Rarity   string
	ItemLvl  int
	Sockets  int
	Quality  int
	Amount   float64
	Currency string
}

// APIError is a non-2xx trade API response.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("trade API HTTP %d: %s", e.Status, e.Body) }

func (c *Client) do(ctx context.Context, lim *Limiter, req *http.Request, out any) error {
	if err := lim.Wait(ctx); err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	lim.Observe(resp)

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := string(body)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return &APIError{Status: resp.StatusCode, Body: msg}
	}
	return json.Unmarshal(body, out)
}

// RunSearch executes a search and returns listing ids (cheapest first).
func (c *Client) RunSearch(ctx context.Context, q *Query) (*SearchResult, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/search/poe2/%s", apiBase, url.PathEscape(c.league))
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var res SearchResult
	if err := c.do(ctx, c.Search, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type fetchResponse struct {
	Result []*struct {
		Listing struct {
			Price *struct {
				Amount   float64 `json:"amount"`
				Currency string  `json:"currency"`
			} `json:"price"`
		} `json:"listing"`
		Item struct {
			BaseType   string `json:"baseType"`
			TypeLine   string `json:"typeLine"`
			Rarity     string `json:"rarity"`
			Ilvl       int    `json:"ilvl"`
			Sockets    []any  `json:"sockets"`
			Properties []struct {
				Name   string  `json:"name"`
				Values [][]any `json:"values"`
			} `json:"properties"`
		} `json:"item"`
	} `json:"result"`
}

var digits = regexp.MustCompile(`\d+`)

// FetchListings fetches up to 10 listings from a search.
func (c *Client) FetchListings(ctx context.Context, searchID string, ids []string) ([]Listing, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 10 {
		ids = ids[:10]
	}
	u := fmt.Sprintf("%s/fetch/%s?query=%s&realm=poe2", apiBase, strings.Join(ids, ","), url.QueryEscape(searchID))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var res fetchResponse
	if err := c.do(ctx, c.Fetch, req, &res); err != nil {
		return nil, err
	}

	var out []Listing
	for _, r := range res.Result {
		if r == nil || r.Listing.Price == nil {
			continue
		}
		l := Listing{
			BaseType: r.Item.BaseType,
			TypeLine: r.Item.TypeLine,
			Rarity:   r.Item.Rarity,
			ItemLvl:  r.Item.Ilvl,
			Sockets:  len(r.Item.Sockets),
			Amount:   r.Listing.Price.Amount,
			Currency: r.Listing.Price.Currency,
		}
		for i, p := range r.Item.Properties {
			// The first property is the item class, e.g. "Boots".
			if i == 0 && len(p.Values) == 0 {
				l.Class = p.Name
			}
			if strings.Contains(p.Name, "Quality") && len(p.Values) > 0 && len(p.Values[0]) > 0 {
				if s, ok := p.Values[0][0].(string); ok {
					l.Quality, _ = strconv.Atoi(digits.FindString(s))
				}
			}
		}
		out = append(out, l)
	}
	return out, nil
}
