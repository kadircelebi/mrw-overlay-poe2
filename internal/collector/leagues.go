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

// MainLeagueCandidate reports whether a league can be the current trade
// league: softcore and not Standard, SSF or Ruthless.
func MainLeagueCandidate(league string) bool {
	low := strings.ToLower(strings.TrimSpace(league))
	return low != "" && low != "standard" && !strings.HasPrefix(low, "hc ") &&
		!strings.Contains(low, "hardcore") && !strings.Contains(low, "ssf") &&
		!strings.Contains(low, "solo self-found") && !strings.Contains(low, "ruthless")
}

// AutoLeague picks the current league from the trade API's list, which puts
// the main league first. The pick sticks: an event league opened mid-league
// (listed before or after the current one) never takes over, because it was
// already on the previous list when the next check runs. The pick moves only
// to a league that is new since the previous list (known) and listed first,
// which is how a new main league shows up at launch. When the current league
// has left the list and nothing new has arrived, it is kept: an ended league
// is not swapped for Standard or an older event league (unless there is no
// history at all, see below).
//
// prev is the league picked last time ("" when none). known is the list seen
// at that time; nil means there is no history, so no league counts as new.
// It returns prev when the list offers nothing better, and "" only when prev
// is "" and the list has no candidate.
func AutoLeague(list, known []string, prev string) string {
	has := func(l []string, name string) bool {
		for _, x := range l {
			if strings.EqualFold(x, name) {
				return true
			}
		}
		return false
	}
	var first string
	for _, l := range list {
		if MainLeagueCandidate(l) {
			first = l
			break
		}
	}
	switch {
	case first == "":
		return prev
	case prev == "":
		return first
	case strings.EqualFold(first, prev):
		return prev
	}
	if known == nil {
		// No history: nothing can be told apart as new. An ended league is
		// still left for the head of the list, the best guess at the current
		// one; otherwise the pick stays.
		if !has(list, prev) {
			return first
		}
		return prev
	}
	if !has(known, first) {
		return first // a league launched since the last check
	}
	return prev
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
