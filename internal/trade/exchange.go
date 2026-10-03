package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ExchangeSeedRules are the bulk exchange limits observed on 2026-10-03
// (policy trade-exchange-request-limit); live headers replace them.
var ExchangeSeedRules = parseRules("5:15:60,10:90:300,30:300:1800")

// ExchangeOffer is one seller's ratio on the bulk exchange: Pay of Currency
// for Get of the wanted item, with Stock of it on hand.
type ExchangeOffer struct {
	Account  string    `json:"account"`
	Currency string    `json:"currency"`
	Pay      float64   `json:"pay"`
	Get      float64   `json:"get"`
	Stock    int       `json:"stock"`
	Indexed  time.Time `json:"indexed"`
	// ValueEx is the price of one item in Exalted Orbs (0 when the currency
	// has no known rate).
	ValueEx float64 `json:"valueEx"`
}

// ExchangeResult is a bulk exchange search, cheapest offer first.
type ExchangeResult struct {
	Want     string          `json:"want"`
	League   string          `json:"league"`
	Total    int             `json:"total"`
	Offers   []ExchangeOffer `json:"offers"`
	TradeURL string          `json:"tradeUrl"`
}

type exchangeResponse struct {
	ID     string `json:"id"`
	Total  int    `json:"total"`
	Result map[string]*struct {
		Listing struct {
			Indexed time.Time `json:"indexed"`
			Account struct {
				Name string `json:"name"`
			} `json:"account"`
			Offers []struct {
				Exchange struct {
					Currency string  `json:"currency"`
					Amount   float64 `json:"amount"`
				} `json:"exchange"`
				Item struct {
					Currency string  `json:"currency"`
					Amount   float64 `json:"amount"`
					Stock    int     `json:"stock"`
				} `json:"item"`
			} `json:"offers"`
		} `json:"listing"`
	} `json:"result"`
}

// Exchange asks the bulk exchange who sells want (a trade static id such as
// "an-audience-with-the-king") for any of have. rateEx converts a payment
// currency id to Exalted Orbs; offers in currencies it does not know keep a
// zero ValueEx and sort last. One call spends one exchange request.
func (c *Client) Exchange(ctx context.Context, league, want string, have []string, status string, rateEx func(string) float64) (ExchangeResult, error) {
	league = strings.TrimSpace(league)
	if league == "" {
		league = c.league
	}
	if status == "" {
		status = "online"
	}
	body := map[string]any{
		"query": map[string]any{
			"status": map[string]string{"option": status},
			"have":   have,
			"want":   []string{want},
		},
		"sort":   map[string]string{"have": "asc"},
		"engine": "new",
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return ExchangeResult{}, err
	}
	u := fmt.Sprintf("%s/exchange/poe2/%s", apiBase, url.PathEscape(league))
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(encoded))
	if err != nil {
		return ExchangeResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var res exchangeResponse
	if err := c.do(ctx, c.ExchangeLimiter(), req, &res); err != nil {
		return ExchangeResult{}, err
	}
	out := ExchangeResult{Want: want, League: league, Total: res.Total, Offers: []ExchangeOffer{}}
	if res.ID != "" {
		out.TradeURL = fmt.Sprintf("https://www.pathofexile.com/trade2/exchange/poe2/%s/%s", url.PathEscape(league), res.ID)
	}
	for _, r := range res.Result {
		if r == nil {
			continue
		}
		for _, o := range r.Listing.Offers {
			if o.Item.Amount <= 0 || o.Exchange.Amount <= 0 || o.Item.Currency != want {
				continue
			}
			offer := ExchangeOffer{
				Account:  r.Listing.Account.Name,
				Currency: o.Exchange.Currency,
				Pay:      o.Exchange.Amount,
				Get:      o.Item.Amount,
				Stock:    o.Item.Stock,
				Indexed:  r.Listing.Indexed,
			}
			if rateEx != nil {
				if rate := rateEx(offer.Currency); rate > 0 {
					offer.ValueEx = offer.Pay / offer.Get * rate
				}
			}
			out.Offers = append(out.Offers, offer)
		}
	}
	sortOffers(out.Offers)
	if out.Total < len(out.Offers) {
		out.Total = len(out.Offers)
	}
	return out, nil
}

// sortOffers puts the cheapest priced offer first and unpriced ones last.
func sortOffers(offers []ExchangeOffer) {
	sort.SliceStable(offers, func(i, j int) bool {
		a, b := offers[i].ValueEx, offers[j].ValueEx
		if (a > 0) != (b > 0) {
			return a > 0
		}
		return a < b
	})
}

// The exchange has its own quota, so it gets its own limiter, made on first
// use: only the overlay ever asks the exchange.
func (c *Client) ExchangeLimiter() *Limiter {
	c.exchangeOnce.Do(func() {
		if c.exchange == nil {
			c.exchange = NewLimiter(c.budget, ExchangeSeedRules)
			c.exchange.SetEvenPacing(false)
		}
	})
	return c.exchange
}
