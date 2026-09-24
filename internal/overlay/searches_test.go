package overlay

import (
	"path/filepath"
	"testing"

	"poe2filter/internal/trade"
)

func TestSearchStoreRoundTrip(t *testing.T) {
	store := NewSearchStore(filepath.Join(t.TempDir(), "overlay_searches.json"))
	items, err := store.Save("Mageblood exact", trade.EvaluateRequest{BaseType: "Utility Belt", Name: "Mageblood", Rarity: "unique"})
	if err != nil || len(items) != 1 {
		t.Fatalf("save = %#v, %v", items, err)
	}
	loaded, err := store.List()
	if err != nil || len(loaded) != 1 || loaded[0].Query.Name != "Mageblood" {
		t.Fatalf("list = %#v, %v", loaded, err)
	}
	loaded, err = store.Delete(loaded[0].ID)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("delete = %#v, %v", loaded, err)
	}
}

func TestSearchStoreRejectsIncompleteSearch(t *testing.T) {
	store := NewSearchStore(filepath.Join(t.TempDir(), "overlay_searches.json"))
	if _, err := store.Save("", trade.EvaluateRequest{BaseType: "Boots"}); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := store.Save("Missing base", trade.EvaluateRequest{}); err == nil {
		t.Fatal("empty base type should fail")
	}
}
