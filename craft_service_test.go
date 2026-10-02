package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"poe2filter/internal/overlay"
	"poe2filter/internal/trade"
)

func TestCraftMarketDoesNotReplaceCapturedItem(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"trade_stats.json":   `{"result":[{"id":"explicit","entries":[{"id":"explicit.mana","text":"# to maximum Mana","type":"explicit"}]}]}`,
		"trade_items.json":   `{"result":[]}`,
		"trade_filters.json": `{"result":[]}`,
		"trade_static.json":  `{"result":[]}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(data, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	s := newAppService(Meta{DataDir: dir})
	captured := &overlay.Item{Raw: "captured item", Class: "Rings", BaseType: "Gold Ring"}
	s.overlaySnapshot = overlay.Snapshot{Item: captured}
	raw := "Item Class: Gloves\nRarity: Rare\nTheoretical Craft\nGloves\n--------\nItem Level: 81\n--------\n{ Prefix Modifier (Tier: 3) }\n+85 to maximum Mana"
	snap, err := s.ParseCraftText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Item.BaseType != "" || snap.Item.Rarity != "rare" || len(snap.Item.Mods) == 0 || snap.Item.Mods[0].StatID != "explicit.mana" {
		t.Fatalf("unexpected craft: %+v", snap.Item)
	}
	// A craft with a picked base searches that base.
	withBase, err := s.ParseCraftText(strings.Replace(raw, "Craft\nGloves\n", "Craft\nSirenscale Gloves\n", 1))
	if err != nil || withBase.Item.BaseType != "Sirenscale Gloves" {
		t.Fatalf("craft base lost: %+v %v", withBase.Item, err)
	}
	q := trade.EvaluateRequest{Rarity: "rare", Stats: []trade.SelectedStat{{ID: "explicit.mana", Min: float64Pointer(85)}}}
	if err := s.ShowCraftMarketWithQuery(raw, q); err != nil {
		t.Fatal(err)
	}
	if s.GetOverlaySnapshot().Item != captured {
		t.Fatal("craft replaced captured item")
	}
	if s.GetMarketSnapshot().Item.Raw != raw || s.GetOverlayDraft().Stats[0].ID != "explicit.mana" {
		t.Fatal("market lost craft or query")
	}
	s.ShowMarketWithQuery(trade.EvaluateRequest{BaseType: "Gold Ring"})
	if s.GetMarketSnapshot().Item != captured {
		t.Fatal("overlay market did not restore captured item")
	}
}

func TestCraftPricesWithoutSnapshotAreUnknown(t *testing.T) {
	got := (&AppService{}).GetCraftPrices()
	if got.Currency == nil || len(got.Currency) != 0 || got.GeneratedAt != "" {
		t.Fatalf("unexpected fallback: %+v", got)
	}
}

func TestCraftClassesMatchTheCraftData(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("frontend", "public", "craft", "data", "classes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Classes []struct {
			ItemClass string `json:"itemClass"`
		} `json:"classes"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range data.Classes {
		seen[c.ItemClass] = true
		if !craftClasses[c.ItemClass] {
			t.Errorf("craft data offers %q but ParseCraftText rejects it", c.ItemClass)
		}
	}
	for c := range craftClasses {
		if !seen[c] {
			t.Errorf("ParseCraftText accepts %q, which the craft data does not offer", c)
		}
	}
}

func TestCraftTextAcceptsCraftClassesOnly(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"trade_stats.json":   `{"result":[{"id":"explicit","entries":[{"id":"explicit.mana","text":"# to maximum Mana","type":"explicit"}]}]}`,
		"trade_items.json":   `{"result":[]}`,
		"trade_filters.json": `{"result":[]}`,
		"trade_static.json":  `{"result":[]}`,
	} {
		if err := os.WriteFile(filepath.Join(data, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	s := newAppService(Meta{DataDir: dir})
	text := func(class string) string {
		return "Item Class: " + class + "\nRarity: Rare\nTheoretical Craft\n" + class +
			"\n--------\nItem Level: 81\n--------\n{ Prefix Modifier (Tier: 3) }\n+85 to maximum Mana"
	}
	snap, err := s.ParseCraftText(text("Rings"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Item.Class != "Rings" || snap.Item.BaseType != "" {
		t.Fatalf("ring craft: %+v", snap.Item)
	}
	if _, err := s.ParseCraftText(text("Jewels")); err == nil {
		t.Fatal("a jewel craft was accepted")
	}
}
