package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// leaguesURL is the league list of the PoE2 trade site, the same API the
// exceptional scanner already talks to.
const leaguesURL = "https://www.pathofexile.com/api/trade2/data/leagues"

// leaguesResponse covers the documented shape, {"result": [...]}, and accepts
// a bare array as well, so a change in the wrapper does not cost the list.
type leaguesResponse struct {
	Result []leagueEntry `json:"result"`
}

func (r *leaguesResponse) UnmarshalJSON(data []byte) error {
	var bare []leagueEntry
	if err := json.Unmarshal(data, &bare); err == nil {
		r.Result = bare
		return nil
	}
	type plain leaguesResponse // no recursion
	return json.Unmarshal(data, (*plain)(r))
}

type leagueEntry struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Realm string `json:"realm"`
}

func (l leagueEntry) name() string {
	if n := strings.TrimSpace(l.ID); n != "" {
		return n
	}
	return strings.TrimSpace(l.Text)
}

// FetchLeagues returns the leagues currently offered by the trade API, in the
// order the API lists them. The response shape is read leniently: an upstream
// change costs the list, never the app, because the caller keeps its previous
// (or built-in) list on error.
func FetchLeagues(ctx context.Context, c *http.Client) ([]string, error) {
	return fetchLeaguesFrom(ctx, c, leaguesURL)
}

func fetchLeaguesFrom(ctx context.Context, c *http.Client, url string) ([]string, error) {
	if c == nil {
		c = NewHTTPClient()
	}
	var resp leaguesResponse
	if err := getJSON(ctx, c, url, &resp); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []string
	for _, e := range resp.Result {
		name := e.name()
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the league list came back empty")
	}
	return out, nil
}
