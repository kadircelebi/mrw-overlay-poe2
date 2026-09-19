package filter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

func testSnapshot() *prices.Snapshot {
	return &prices.Snapshot{
		SchemaVersion: prices.SchemaVersion,
		League:        "Test",
		GeneratedAt:   time.Now(),
		Rates:         prices.Rates{DivineEx: 400, ChaosEx: 50},
		Currency: []prices.CurrencyPrice{
			{Name: "Mirror of Kalandra", Category: "currency", ValueEx: 1_500_000},
			{Name: "Orb of Alchemy", Category: "currency", ValueEx: 1},
		},
		UniqueBases: map[string]prices.UniqueBase{
			"Silk Robe": {TopName: "Temporalis", MaxEx: 2_000_000, Uniques: []prices.Unique{
				{Name: "Temporalis", ValueEx: 2_000_000, Listings: 15},
				{Name: "Cloak of Flame", ValueEx: 1, Listings: 1200},
			}},
			"Knight Armour": {TopName: "The Sunken Vessel", MaxEx: 1, Uniques: []prices.Unique{
				{Name: "The Sunken Vessel", ValueEx: 1, Listings: 269},
			}},
			// Thin data: one listing, must never be hidden.
			"Moulded Mitts": {TopName: "Hateforge", MaxEx: 2, Uniques: []prices.Unique{
				{Name: "Hateforge", ValueEx: 2, Listings: 1},
			}},
		},
		Exceptional: []prices.ExceptionalPrice{
			{Base: "Sekhema Sandals", Kind: prices.KindSockets, Min: 2, ValueEx: 300, Listings: 40, Samples: 8},
			{Base: "Cavalry Boots", Kind: prices.KindSockets, Min: 2, ValueEx: 5, Listings: 700, Samples: 8},
			// Too few listings to trust a hide.
			{Base: "Adherent Cuffs", Kind: prices.KindSockets, Min: 2, ValueEx: 4, Listings: 3, Samples: 3},
		},
	}
}

var testBases = map[string]string{
	"mirror of kalandra": "Mirror of Kalandra", "orb of alchemy": "Orb of Alchemy",
	"silk robe": "Silk Robe", "knight armour": "Knight Armour", "moulded mitts": "Moulded Mitts",
	"sekhema sandals": "Sekhema Sandals", "cavalry boots": "Cavalry Boots",
	"adherent cuffs": "Adherent Cuffs", "heavy belt": "Heavy Belt",
}

// blockContaining returns the index of the first rule block whose text
// contains all needles.
func blockContaining(t *testing.T, out string, needles ...string) int {
	t.Helper()
	for i, blk := range strings.Split(out, "\n\n") {
		ok := true
		for _, n := range needles {
			if !strings.Contains(blk, n) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func TestRuleOrderAndSafety(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinValue, cfg.MinValueUnit = 10, "exalted"
	cfg.Blacklist = []string{"Mirror of Kalandra"}
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases)

	black := blockContaining(t, out, "Hide", `"Mirror of Kalandra"`)
	white := blockContaining(t, out, "Show", `"Mirror of Kalandra"`)
	if black < 0 || (white >= 0 && black > white) {
		t.Fatalf("blacklist must come before any Show of the same item (black=%d show=%d)", black, white)
	}

	valuable := blockContaining(t, out, "Show", "Sockets >= 2", `"Sekhema Sandals"`)
	cheap := blockContaining(t, out, "Hide", "Sockets >= 2", `"Cavalry Boots"`)
	unknown := blockContaining(t, out, "Show", "Sockets >= 2", `Class == `, `"Boots"`)
	blanket := blockContaining(t, out, "Hide", "Rarity Normal Magic Rare", `"Boots"`)
	if !(valuable >= 0 && valuable < cheap && cheap < unknown && unknown < blanket) {
		t.Fatalf("exceptional order wrong: valuable=%d cheap=%d unknown=%d blanket=%d", valuable, cheap, unknown, blanket)
	}
	if blockContaining(t, out, "Hide", `"Adherent Cuffs"`) >= 0 {
		t.Fatal("exceptional base with too few listings must not be hidden")
	}

	if blockContaining(t, out, "Show", "Rarity Unique", `"Silk Robe"`) < 0 {
		t.Fatal("Silk Robe (Temporalis) must be shown")
	}
	if blockContaining(t, out, "Hide", "Rarity Unique", `"Knight Armour"`) < 0 {
		t.Fatal("well-traded cheap unique base should be hidden")
	}
	if blockContaining(t, out, "Hide", `"Moulded Mitts"`) >= 0 {
		t.Fatal("unique base priced by a single listing must not be hidden")
	}
	if !strings.Contains(out, "GemLevel >= 20") || strings.Contains(out, `"Tablet"`) {
		t.Fatal("uncut gems must use GemLevel and no invalid Tablet class")
	}
	if blockContaining(t, out, "UnidentifiedItemTier >= 5", `"Rings"`) < 0 {
		t.Fatal("T5 rare equipment rule missing")
	}
	if st.ValuableExcept != 1 || st.CheapExcept != 1 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestBlacklistProtectsValuableSibling(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Blacklist = []string{"Cloak of Flame"} // shares Silk Robe with Temporalis
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases)
	if blockContaining(t, out, "Hide", "Rarity Unique", `"Silk Robe"`) >= 0 {
		t.Fatal("blacklisting a junk unique must not hide Temporalis' base")
	}
	if len(st.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

func TestShowOnlyNeverHidesByValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FilterMode = "show_only"
	cfg.IncludeGear = false
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases)
	for _, blk := range strings.Split(out, "\n\n") {
		if strings.Contains(blk, "\nHide") || strings.HasPrefix(strings.TrimSpace(blk), "Hide") {
			if !strings.Contains(blk, "Uncut") {
				t.Fatalf("unexpected hide in show_only mode:\n%s", blk)
			}
		}
	}
}

func TestThresholdUnits(t *testing.T) {
	r := prices.Rates{DivineEx: 400, ChaosEx: 50}
	for _, tc := range []struct {
		unit string
		want float64
	}{{"exalted", 2}, {"chaos", 100}, {"divine", 800}} {
		c := Config{MinValue: 2, MinValueUnit: tc.unit}
		if got := c.ThresholdEx(r); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.unit, got, tc.want)
		}
	}
}

func TestLegacyConfigMigration(t *testing.T) {
	// The config file that shipped with the old version.
	legacy := `{"min_exalt": 10, "min_divine": 1, "filter_mode": "hide", "include_gear": true,
	 "only_3_sockets": true, "t5_jewels_only": true, "hide_exalt": true, "filter_name": "auto_updated",
	 "blacklist": ["Scroll of Wisdom", "Exalted Orb"], "base_filter_preset": "base.filter",
	 "auto_update_enabled": false, "auto_update_interval": 30, "league_name": "Forbidden Rites"}`
	p := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := LoadConfig(p)
	if c.MinValue != 10 || c.MinValueUnit != "exalted" {
		t.Errorf("threshold: %v %s", c.MinValue, c.MinValueUnit)
	}
	if c.Strictness != 6 || c.CustomBaseFilter != "" {
		t.Errorf("strictness: %d custom=%q", c.Strictness, c.CustomBaseFilter)
	}
	if !c.HideExalt || len(c.Blacklist) != 2 || c.AutoUpdateEnabled {
		t.Errorf("user choices not kept: %+v", c)
	}
	if c.AutoUpdateHours != 1 || !c.T5Rares || !c.ExceptionalScan {
		t.Errorf("new defaults: hours=%d t5=%v scan=%v", c.AutoUpdateHours, c.T5Rares, c.ExceptionalScan)
	}
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "min_exalt") || strings.Contains(string(data), "base_filter_preset") {
		t.Error("legacy keys must not be written back")
	}
}

func TestInjectKeepsDollarSigns(t *testing.T) {
	base := "#header\n# [[0100]] OVERRIDE AREA 1 - Override ALL rules here\n\n#=====\n# [[0100]] Gold\n#=====\nShow # $type->x $tier->y\n"
	block := "#=====\n# [[DYNAMIC LOOT FILTER]]\nShow # $keep->this\n"
	out := Inject(base, block)
	if !strings.Contains(out, "$keep->this") || !strings.Contains(out, "$type->x $tier->y") {
		t.Fatalf("dollar signs mangled:\n%s", out)
	}
	if strings.Index(out, "DYNAMIC") > strings.Index(out, "[[0100]] Gold") || strings.Index(out, "DYNAMIC") < strings.Index(out, "#header") {
		t.Fatal("block must sit after the header and before the first section")
	}
	// Re-injecting into our own output must not duplicate the block.
	again := Inject(out, block)
	if strings.Count(again, "[[DYNAMIC LOOT FILTER]]") != 1 {
		t.Fatal("block duplicated on re-inject")
	}
}

func TestWhitelistUniqueOnlyAndChanceNormal(t *testing.T) {
	bases := map[string]string{"sapphire": "Sapphire", "heavy belt": "Heavy Belt", "silk robe": "Silk Robe"}
	cfg := DefaultConfig()
	cfg.Whitelist = []string{"Sapphire|unique", "Silk Robe"}
	cfg.ChanceBases = []string{"Heavy Belt"}
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), bases)

	if blockContaining(t, out, "Show", "Rarity Unique", `"Sapphire"`) < 0 {
		t.Fatal("unique-only whitelist entry must be restricted to uniques")
	}
	for _, blk := range strings.Split(out, "\n\n") {
		if strings.Contains(blk, `"Sapphire"`) && !strings.Contains(blk, "Rarity Unique") {
			t.Fatalf("Sapphire shown for all rarities:\n%s", blk)
		}
	}
	if blockContaining(t, out, "Show", `"Silk Robe"`) < 0 {
		t.Fatal("plain whitelist entry missing")
	}
	if blockContaining(t, out, "Rarity Normal", `"Heavy Belt"`) < 0 || blockContaining(t, out, "Rarity Normal Magic", `"Heavy Belt"`) >= 0 {
		t.Fatal("chance bases must be normal rarity only")
	}
}
