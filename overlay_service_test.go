package main

import (
	"encoding/json"
	"testing"
	"time"

	"poe2filter/internal/trade"
)

func TestEvaluateOverlayUsesFreshMemoryCache(t *testing.T) {
	query := trade.EvaluateRequest{
		League:   "Test League",
		BaseType: "Utility Belt",
		Rarity:   "unique",
		Filters: []trade.SelectedFilter{
			{Group: "misc_filters", ID: "sanctified", Option: "true"},
			{Group: "type_filters", ID: "ilvl", Min: float64Pointer(80)},
		},
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	want := trade.Evaluation{SearchID: "cached-search", Total: 7}
	service := &AppService{
		overlayEvalCache: map[string]overlayEvaluationCacheEntry{
			string(encoded): {result: want, expiresAt: time.Now().Add(time.Minute)},
			"expired":       {expiresAt: time.Now().Add(-time.Second)},
		},
		overlayEvalFlights: make(map[string]*overlayEvaluationFlight),
	}

	got, err := service.EvaluateOverlay(query, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.SearchID != want.SearchID || got.Total != want.Total {
		t.Fatalf("cached result = %+v, want %+v", got, want)
	}
	if _, exists := service.overlayEvalCache["expired"]; exists {
		t.Fatal("expired cache entry was not pruned")
	}
}

func float64Pointer(value float64) *float64 { return &value }
