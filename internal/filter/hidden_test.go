package filter

import (
	"strings"
	"testing"

	"poe2filter/internal/filtereval"
	"poe2filter/internal/prices"
)

// "While cheap" entries sit behind the valuable-item rules (a Mirror on the
// list is still shown), "always" entries above them; exotic bases are never
// hidden; rarity and stack limits become conditions.
func TestHiddenItems(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Exotics = []string{"Show # %D8 $type->exoticbases $tier->pseudocrafts2 !exotics_ctier\n\tRarity Normal Magic Rare\n\tBaseType == \"Lament Amulet\""}
	cfg.HiddenItems = []HiddenItem{
		{Base: "Mirror of Kalandra", WhileCheap: true},
		{Base: "Orb of Alchemy", BelowStack: 5, WhileCheap: true},
		{Base: "Prismatic Ring", Rarities: []string{"rare"}},
		{Base: "Lament Amulet"},
		{Base: "Prismatic Ring", Rarities: []string{"Rare"}}, // same entry again
	}
	cfg.Normalize()
	if len(cfg.HiddenItems) != 4 {
		t.Fatalf("normalised list = %+v", cfg.HiddenItems)
	}
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	mirrorShow := blockContaining(t, out, "Show", `"Mirror of Kalandra"`)
	mirrorHide := blockContaining(t, out, "Hide", `"Mirror of Kalandra"`)
	if mirrorHide < 0 || mirrorShow < 0 || mirrorHide < mirrorShow {
		t.Errorf("while-cheap Mirror: show %d, hide %d (hide must come after)", mirrorShow, mirrorHide)
	}
	if blockContaining(t, out, "Hide", "StackSize < 5", `"Orb of Alchemy"`) < 0 {
		t.Error("stack limit missing")
	}
	ring := blockContaining(t, out, "Hide", "Rarity Rare", `"Prismatic Ring"`)
	if ring < 0 || ring > mirrorShow {
		t.Errorf("always-hidden ring at %d, want before the valuable rules (%d)", ring, mirrorShow)
	}
	if blockContaining(t, out, "Hide", `"Lament Amulet"`) >= 0 {
		t.Error("exotic base hidden")
	}
	warned := false
	for _, w := range st.Warnings {
		warned = warned || strings.Contains(w, "Lament Amulet")
	}
	if !warned {
		t.Errorf("no warning about the exotic base: %v", st.Warnings)
	}

	cfg.HiddenOff = true
	out, _ = GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	if blockContaining(t, out, "Hide", `"Prismatic Ring"`) >= 0 {
		t.Error("hidden list written while switched off")
	}
}

func TestHiddenUniqueOverridesShowRulesUnlessWhileCheap(t *testing.T) {
	for _, cheap := range []bool{false, true} {
		cfg := DefaultConfig()
		cfg.MinValue, cfg.MinValueUnit = 100, "exalted"
		cfg.HiddenItems = []HiddenItem{{Base: "Gold Ring", Rarities: []string{"Unique"}, WhileCheap: cheap}}
		cfg.ItemGroups = []ItemGroup{
			{ID: "show", Name: "Always show", Items: []string{"Gold Ring"}, Always: true},
			{ID: "value", Name: "Value tier", Mode: ItemGroupModeValue, ThresholdValue: 100, ThresholdUnit: "exalted"},
		}
		snap := testSnapshot()
		snap.UniqueBases["Gold Ring"] = prices.UniqueBase{TopName: "Example Ring", MaxEx: 500,
			Uniques: []prices.Unique{{Name: "Example Ring", ValueEx: 500, Listings: 20}}}
		cfg.Normalize()
		out, _ := GenerateDynamicFilterBlock(cfg, snap, map[string]string{"gold ring": "Gold Ring"}, nil)
		result := filtereval.Evaluate(filtereval.Parse(out), filtereval.Facts{Class: "Rings", BaseType: "Gold Ring", Rarity: "Unique"})
		want := filtereval.Hide
		if cheap {
			want = filtereval.Show
		}
		if result.Final == nil || result.Final.Block.Action != want {
			t.Fatalf("whileCheap=%v: result=%+v, want %s", cheap, result, want)
		}
	}
}
