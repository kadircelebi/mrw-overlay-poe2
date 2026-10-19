package invscan

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLocalCDNIcons loads the exchange icons the trade site lists in
// INVSCAN_STATIC (a trade_static.json) from GGG's image server into a new
// folder, then once more from that folder.
func TestLocalCDNIcons(t *testing.T) {
	path := os.Getenv("INVSCAN_STATIC")
	if path == "" {
		t.Skip("INVSCAN_STATIC not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var static struct {
		Result []struct {
			ID      string                             `json:"id"`
			Entries []struct{ ID, Text, Image string } `json:"entries"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &static); err != nil {
		t.Fatal(err)
	}
	var sources []Source
	for _, g := range static.Result {
		if !InScope(g.ID) {
			continue
		}
		for _, e := range g.Entries {
			if e.Image == "" {
				continue
			}
			u := e.Image
			if strings.HasPrefix(u, "/") {
				u = "https://web.poecdn.com" + u
			}
			sources = append(sources, Source{Entry{ID: e.ID, Name: e.Text, Group: g.ID}, u})
		}
	}
	cache := IconCache{Dir: t.TempDir(), Workers: 2, Pause: 50 * time.Millisecond}
	start := time.Now()
	icons, err := cache.Load(context.Background(), sources)
	t.Logf("%d items, %d icons downloaded in %v (err %v)", len(sources), len(icons), time.Since(start), err)
	start = time.Now()
	again, err := cache.Load(context.Background(), sources)
	t.Logf("again from disk: %d icons in %v (err %v)", len(again), time.Since(start), err)
}
