package trade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The trade site's sales history: the last sales of the signed-in account's
// merchant (Ange), newest first. It answers only with a session and has a
// tight quota of its own, counted per account.

// HistorySeedRules are the account limits seen on 2026-10-15
// (X-Rate-Limit-Policy: trade-history-request-limit).
var HistorySeedRules = parseRules("5:60:60,10:600:120,15:10800:3600")

// ErrSignedOut means the history needs a pathofexile.com session and there
// is none, or the stored one has expired.
var ErrSignedOut = errors.New("pathofexile.com session is missing or expired")

// HistoryWaitError reports that the history quota has no room right now.
type HistoryWaitError struct{ In time.Duration }

func (e *HistoryWaitError) Error() string {
	return fmt.Sprintf("trade history quota: next request in %s", e.In.Round(time.Second))
}

// HistorySale is one sale as the trade site lists it.
type HistorySale struct {
	ItemID   string      `json:"itemId"`
	Time     time.Time   `json:"time"`
	Amount   float64     `json:"amount"`
	Currency string      `json:"currency"`
	Item     HistoryItem `json:"item"`
}

// HistoryItem is what is kept of a sold item: enough to show and recognise
// it, not the whole listing.
type HistoryItem struct {
	Name      string   `json:"name,omitempty"`
	TypeLine  string   `json:"typeLine"`
	BaseType  string   `json:"baseType,omitempty"`
	Rarity    string   `json:"rarity,omitempty"`
	Icon      string   `json:"icon,omitempty"`
	ItemLevel int      `json:"ilvl,omitempty"`
	Corrupted bool     `json:"corrupted,omitempty"`
	Mods      []string `json:"mods,omitempty"`
}

// historyMod is a mod line, which the site sends either as text or as an
// object with the text under "description".
type historyMod struct {
	Description string
}

func (m *historyMod) UnmarshalJSON(raw []byte) error {
	if json.Unmarshal(raw, &m.Description) == nil {
		return nil
	}
	var obj struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	m.Description = obj.Description
	return nil
}

type historyAnswer struct {
	Result []struct {
		ItemID string `json:"item_id"`
		Time   string `json:"time"`
		Price  struct {
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		} `json:"price"`
		Item struct {
			Name         string       `json:"name"`
			TypeLine     string       `json:"typeLine"`
			BaseType     string       `json:"baseType"`
			Rarity       string       `json:"rarity"`
			Icon         string       `json:"icon"`
			ItemLevel    int          `json:"ilvl"`
			Corrupted    bool         `json:"corrupted"`
			EnchantMods  []historyMod `json:"enchantMods"`
			ImplicitMods []historyMod `json:"implicitMods"`
			ExplicitMods []historyMod `json:"explicitMods"`
			RuneMods     []historyMod `json:"runeMods"`
		} `json:"item"`
	} `json:"result"`
}

// HistoryNextIn is how long until the history quota allows a request.
func (c *Client) HistoryNextIn() time.Duration {
	return c.History.NextIn()
}

// TradeHistory fetches the account's latest sales in league. It never waits
// for the quota: when there is no room it returns a *HistoryWaitError.
func (c *Client) TradeHistory(ctx context.Context, league string) ([]HistorySale, error) {
	if c.sessionValue() == "" {
		return nil, ErrSignedOut
	}
	if wait := c.History.NextIn(); wait > 0 {
		return nil, &HistoryWaitError{In: wait}
	}
	u := fmt.Sprintf("%s/history/%s", apiBase, url.PathEscape(league))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var answer historyAnswer
	if err := c.do(ctx, c.History, req, &answer); err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == http.StatusUnauthorized || api.Status == http.StatusForbidden) && !api.Blocked() {
			return nil, ErrSignedOut
		}
		return nil, err
	}
	sales := make([]HistorySale, 0, len(answer.Result))
	for _, r := range answer.Result {
		at, err := time.Parse(time.RFC3339, r.Time)
		if err != nil || r.ItemID == "" {
			continue
		}
		it := r.Item
		item := HistoryItem{Name: it.Name, TypeLine: it.TypeLine, BaseType: it.BaseType, Rarity: it.Rarity, Icon: it.Icon, ItemLevel: it.ItemLevel, Corrupted: it.Corrupted}
		for _, group := range [][]historyMod{it.EnchantMods, it.ImplicitMods, it.RuneMods, it.ExplicitMods} {
			for _, m := range group {
				if text := PlainModText(m.Description); text != "" {
					item.Mods = append(item.Mods, text)
				}
			}
		}
		sales = append(sales, HistorySale{ItemID: r.ItemID, Time: at.UTC(), Amount: r.Price.Amount, Currency: r.Price.Currency, Item: item})
	}
	return sales, nil
}

// modMarkup is the trade site's tag markup in mod text: "[Spell]" or
// "[AoESkill|AoE]", shown as its last part.
var modMarkup = regexp.MustCompile(`\[([^\[\]|]*\|)?([^\[\]|]*)\]`)

// PlainModText drops the tag markup from a mod line.
func PlainModText(s string) string {
	return strings.TrimSpace(modMarkup.ReplaceAllString(s, "$2"))
}
