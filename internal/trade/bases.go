package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"poe2filter/internal/prices"
	"poe2filter/internal/useragent"
)

type itemsData struct {
	Result []struct {
		ID      string `json:"id"`
		Entries []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Flags *struct {
				Unique bool `json:"unique"`
			} `json:"flags"`
		} `json:"entries"`
	} `json:"result"`
}

// EquipmentBaseTypes returns every non-unique armour and weapon base from the
// trade API's static item list, cached on disk for a day.
func EquipmentBaseTypes(ctx context.Context, cachePath string) ([]string, error) {
	var raw []byte
	if fi, err := os.Stat(cachePath); err == nil && time.Since(fi.ModTime()) < 24*time.Hour {
		raw, _ = os.ReadFile(cachePath)
	}
	if raw == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/data/items", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", useragent.Value())
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				raw, err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
			} else {
				err = fmt.Errorf("HTTP %d", resp.StatusCode)
			}
		}
		if raw != nil && err == nil {
			_ = prices.WriteFileAtomic(cachePath, raw)
		} else if cached, cerr := os.ReadFile(cachePath); cerr == nil {
			raw = cached // stale cache beats nothing
		} else {
			return nil, fmt.Errorf("could not fetch the trade base list: %w", err)
		}
	}

	var data itemsData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, group := range data.Result {
		if group.ID != "armour" && group.ID != "weapon" {
			continue
		}
		for _, e := range group.Entries {
			if e.Name != "" || (e.Flags != nil && e.Flags.Unique) {
				continue
			}
			if strings.HasPrefix(e.Type, "Runeforged ") || strings.HasPrefix(e.Type, "Runemastered ") {
				continue // unique-only variants, never dropped as normal items
			}
			if !seen[e.Type] {
				seen[e.Type] = true
				out = append(out, e.Type)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// BuildCandidates keeps bases the loot filter knows (i.e. that actually drop)
// and ranks bases from the preferred set first.
func BuildCandidates(bases []string, droppable map[string]bool, preferred map[string]bool) []Candidate {
	var out []Candidate
	for _, b := range bases {
		if len(droppable) > 0 && !droppable[b] {
			continue
		}
		p := 1
		if preferred[b] {
			p = 0
		}
		out = append(out, Candidate{Base: b, Priority: p})
	}
	return out
}
