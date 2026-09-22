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
	cfg.ItemGroups = []ItemGroup{{ID: "g1", Name: "Hide", Items: []string{"Mirror of Kalandra"}, Hide: true}}
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

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
	// Cloak of Flame shares Silk Robe with Temporalis.
	cfg.ItemGroups = []ItemGroup{{ID: "g1", Name: "Hide", Items: []string{"Cloak of Flame"}, Hide: true}}
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
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
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	for _, blk := range strings.Split(out, "\n\n") {
		if !strings.Contains(blk, "\nHide") && !strings.HasPrefix(strings.TrimSpace(blk), "Hide") {
			continue
		}
		// The tier sliders are explicit choices ("show 5+, hide the rest"), so
		// they still hide; what show_only forbids is hiding because of price.
		if strings.Contains(blk, "Uncut") || strings.Contains(blk, `Class == "Jewels"`) {
			continue
		}
		t.Fatalf("unexpected hide in show_only mode:\n%s", blk)
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

func TestUserValueGroupsUseHighestConvertedThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinValue, cfg.MinValueUnit = 75, "exalted"
	// Deliberately not in price order: generation must sort by converted value.
	cfg.ItemGroups = []ItemGroup{
		{ID: "g2", Name: "One Divine", Mode: ItemGroupModeValue, ThresholdValue: 1, ThresholdUnit: "divine"},
		{ID: "g1", Name: "Three Chaos", Mode: ItemGroupModeValue, ThresholdValue: 3, ThresholdUnit: "chaos"},
		{ID: "g3", Name: "Ten Divine", Mode: ItemGroupModeValue, ThresholdValue: 10, ThresholdUnit: "divine"},
	}
	cfg.Styles = map[string]string{"user:g1": "neon_green", "user:g3": "neon_red"}
	snap := testSnapshot()
	snap.Currency = append(snap.Currency,
		prices.CurrencyPrice{Name: "Orb of Annulment", Category: "currency", ValueEx: 250},
		prices.CurrencyPrice{Name: "Divine Orb", Category: "currency", ValueEx: 400},
		prices.CurrencyPrice{Name: "Perfect Exalted Orb", Category: "currency", ValueEx: 550},
	)
	bases := map[string]string{}
	for k, v := range testBases {
		bases[k] = v
	}
	for _, name := range []string{"Orb of Annulment", "Divine Orb", "Perfect Exalted Orb"} {
		bases[strings.ToLower(name)] = name
	}

	out, st := GenerateDynamicFilterBlock(cfg, snap, bases, nil)
	g3 := blockContaining(t, out, "TEN DIVINE", `"Mirror of Kalandra"`)
	g2 := blockContaining(t, out, "ONE DIVINE", `"Divine Orb"`, `"Perfect Exalted Orb"`)
	g1 := blockContaining(t, out, "THREE CHAOS", `"Orb of Annulment"`)
	if g3 < 0 || g2 < 0 || g1 < 0 || !(g3 < g2 && g2 < g1) {
		t.Fatalf("value groups not emitted from highest to lowest: g3=%d g2=%d g1=%d\n%s", g3, g2, g1, out)
	}
	if blockContaining(t, out, "THREE CHAOS", `"Mirror of Kalandra"`) >= 0 {
		t.Fatal("a high-value item must belong only to the highest threshold it reaches")
	}
	whitelistMirror := blockContaining(t, out, `"Mirror of Kalandra"`, "SetBackgroundColor 180 0 0 255")
	if whitelistMirror < 0 || g3 > whitelistMirror {
		t.Fatalf("value group must win over the default Mirror whitelist style: tier=%d whitelist=%d", g3, whitelistMirror)
	}
	if blockContaining(t, out, "Sockets >= 2", `"Sekhema Sandals"`, "SetBackgroundColor "+themeByID["neon_green"].BgColor) < 0 {
		t.Fatal("priced exceptional items must use value groups too")
	}
	if blockContaining(t, out, "Rarity Unique", `"Silk Robe"`, "SetBackgroundColor "+themeByID["neon_red"].BgColor) < 0 {
		t.Fatal("priced unique bases must use value groups too")
	}
	// The value-group rule must precede the legacy dedicated Divine rule.
	legacyDivine := blockContaining(t, out, `Class == "Stackable Currency"`, `BaseType == "Divine Orb"`)
	if legacyDivine < 0 || g2 > legacyDivine {
		t.Fatalf("value group must win over the legacy Divine style: tier=%d legacy=%d", g2, legacyDivine)
	}
	if st.ValuableCurrency != 4 || st.ValuableUniques != 1 || st.ValuableExcept != 1 {
		t.Fatalf("tiered items missing from stats: %+v", st)
	}
}

func TestValueGroupAtOrBelowBaseThresholdIsIgnored(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinValue, cfg.MinValueUnit = 75, "exalted"
	cfg.ItemGroups = []ItemGroup{{
		ID: "g1", Name: "Too low", Mode: ItemGroupModeValue,
		ThresholdValue: 1, ThresholdUnit: "chaos", // 50 Exalted in test rates
	}}
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	if strings.Contains(out, "TOO LOW") {
		t.Fatal("a value group below the base threshold must not replace the base appearance")
	}
	if len(st.Warnings) != 1 || !strings.Contains(st.Warnings[0], "Too low") {
		t.Fatalf("expected a useful warning, got %v", st.Warnings)
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
	if !c.HideExalt || len(c.ItemGroups) != 1 || len(c.ItemGroups[0].Items) != 2 || c.AutoUpdateEnabled {
		t.Errorf("user choices not kept: %+v", c)
	}
	if c.AutoUpdateHours != 1 || c.T5RareTier != MaxRareTier || !c.ExceptionalScan {
		t.Errorf("new defaults: hours=%d t5=%d scan=%v", c.AutoUpdateHours, c.T5RareTier, c.ExceptionalScan)
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

// Filters written by the Turkish build up to v1.2.0 carry a translated opening
// marker. If such a file is fed back in as a custom base, the old block still
// has to be stripped instead of stacking up on every update.
func TestInjectStripsLegacyTurkishMarker(t *testing.T) {
	base := "#header\n#=====\n# [[DİNAMİK LOOT FİLTRESİ]] - poe2-filter\nShow # old\n# [[END DYNAMIC LOOT FILTER]]\n\n#=====\n# [[0100]] Gold\n#=====\nShow\n"
	out := Inject(base, "#=====\n# [[DYNAMIC LOOT FILTER]]\nShow # new\n")
	if strings.Contains(out, "DİNAMİK") || strings.Contains(out, "Show # old") {
		t.Fatalf("the old block should be gone:\n%s", out)
	}
	if strings.Count(out, "[[END DYNAMIC LOOT FILTER]]") != 1 {
		t.Fatalf("exactly one block expected:\n%s", out)
	}
}

func TestWhitelistUniqueOnlyAndChanceNormal(t *testing.T) {
	bases := map[string]string{"sapphire": "Sapphire", "heavy belt": "Heavy Belt", "silk robe": "Silk Robe"}
	cfg := DefaultConfig()
	cfg.Whitelist = []string{"Sapphire|unique", "Silk Robe"}
	cfg.ChanceBases = []string{"Heavy Belt"}
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), bases, nil)

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

func TestStyleGroupsApplyAndMigrate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DivineTheme = "dark" // legacy field only
	cfg.Styles = map[string]string{GroupUnique: "neon_green", GroupCurrency: DefaultThemeID, "bogus": "neon_red", GroupT5Rare: "nope"}
	cfg.Normalize()
	if cfg.Styles[GroupDivine] != "dark" || cfg.DivineTheme != "dark" {
		t.Fatalf("divine theme not migrated: %v", cfg.Styles)
	}
	if _, ok := cfg.Styles["bogus"]; ok {
		t.Fatal("unknown group kept")
	}
	if _, ok := cfg.Styles[GroupT5Rare]; ok {
		t.Fatal("unknown theme kept")
	}
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	green := themeByID["neon_green"]
	if blockContaining(t, out, "Rarity Unique", `"Silk Robe"`, "SetBackgroundColor "+green.BgColor, "PlayEffect Green", "MinimapIcon 0 Green Star") < 0 {
		t.Fatalf("unique group palette not applied:\n%s", out)
	}
	dark := themeByID["dark"]
	if blockContaining(t, out, `"Divine Orb"`, "SetBackgroundColor "+dark.BgColor) < 0 {
		t.Fatal("divine palette not applied")
	}
	// Defaults reproduce the built-in look exactly.
	def, _ := DefaultConfig().Palette(GroupWhitelist, nil)
	if *styleMax.with(def) != *styleMax {
		t.Fatal("default whitelist palette differs from built-in style")
	}
}

func TestMediumWhitelistOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ItemGroups = []ItemGroup{{ID: "g1", Name: "Mid", Items: []string{"Orb of Alchemy", "Silk Robe|unique"}}}
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	mid := blockContaining(t, out, "Show", `"Orb of Alchemy"`, "MinimapIcon 1 Purple Diamond")
	hide := blockContaining(t, out, "Hide", `"Orb of Alchemy"`)
	if mid < 0 || (hide >= 0 && hide < mid) {
		t.Fatalf("medium list must show a cheap item before it is hidden (mid=%d hide=%d)", mid, hide)
	}
	strong := blockContaining(t, out, "Show", "Rarity Unique", `"Silk Robe"`, "PlayEffect Red")
	midSilk := blockContaining(t, out, "Show", "Rarity Unique", `"Silk Robe"`, "Purple Diamond")
	if strong < 0 || midSilk < 0 || strong > midSilk {
		t.Fatalf("a valuable item must keep its stronger highlight (strong=%d mid=%d)", strong, midSilk)
	}
}

func TestGroupSounds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sounds = map[string]string{
		GroupDivine: "1", GroupUnique: SoundNone, GroupT5Rare: SoundFilePrefix + "nebu.mp3",
		GroupChance: "99", "bogus": "2",
	}
	cfg.Normalize()
	if _, ok := cfg.Sounds[GroupChance]; ok {
		t.Fatal("invalid sound id kept")
	}
	if _, ok := cfg.Sounds["bogus"]; ok {
		t.Fatal("unknown group kept")
	}
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	divine := strings.Split(out, "\n\n")[blockContaining(t, out, `"Divine Orb"`, "SetFontSize 45")]
	if !strings.Contains(divine, "PlayAlertSound 1 300") || strings.Contains(divine, "PlayAlertSound 6") {
		t.Fatalf("divine sound 1 not applied:\n%s", divine)
	}
	unique := strings.Split(out, "\n\n")[blockContaining(t, out, "Show", "Rarity Unique", `"Silk Robe"`)]
	if strings.Contains(unique, "AlertSound") {
		t.Fatalf("silenced group still plays a sound:\n%s", unique)
	}
	if blockContaining(t, out, "UnidentifiedItemTier >= 5", `CustomAlertSound "nebu.mp3" 300`) < 0 {
		t.Fatal("custom sound file not applied")
	}

	legacy := DefaultConfig()
	legacy.DivineSound = "nebu.mp3"
	legacy.Normalize()
	if legacy.Sounds[GroupDivine] != SoundFilePrefix+"nebu.mp3" || legacy.DivineSound != "" {
		t.Fatalf("legacy divine sound not migrated: %v", legacy.Sounds)
	}
}

func TestUncutGemSliders(t *testing.T) {
	// Support gems on the hide stop: hidden whatever the skill gem slider says.
	for _, lvl := range []int{TierOff, 20} {
		cfg := DefaultConfig()
		cfg.UncutGemLevel, cfg.UncutSupportLevel = lvl, TierHide
		out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
		if blockContaining(t, out, "Hide", `BaseType "Uncut Support Gem"`) < 0 {
			t.Fatalf("skill=%d: support gems must be hidden on the hide stop", lvl)
		}
		if blockContaining(t, out, "Show", `"Uncut Support Gem"`) >= 0 {
			t.Fatalf("skill=%d: support gems must not be shown on the hide stop", lvl)
		}
	}

	// Each slider writes its own threshold.
	cfg := DefaultConfig()
	cfg.UncutGemLevel, cfg.UncutSupportLevel = 20, 3
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	if blockContaining(t, out, "Show", `"Uncut Skill Gem"`, "GemLevel >= 20") < 0 {
		t.Error("skill gems should be shown from level 20")
	}
	if blockContaining(t, out, "Show", `"Uncut Support Gem"`, "GemLevel >= 3") < 0 {
		t.Error("support gems should follow their own level")
	}
	if blockContaining(t, out, "Hide", `BaseType "Uncut Skill Gem" "Uncut Spirit Gem"`) < 0 {
		t.Error("gems below the level must still be hidden")
	}

	// Both off: no uncut rule at all except the support hide.
	cfg.UncutGemLevel, cfg.UncutSupportLevel = TierOff, 1
	out, _ = GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	if blockContaining(t, out, "Hide", `BaseType "Uncut Skill Gem" "Uncut Spirit Gem"`) >= 0 {
		t.Error("an off slider must not write a rule for skill gems")
	}
	if blockContaining(t, out, "Show", `"Uncut Support Gem"`, "GemLevel >= 1") < 0 {
		t.Error("support gems at 1+ should be shown at any level")
	}
}

// An uncut gem's base type carries its level ("Uncut Support Gem (Level 5)"),
// so an exact match never fires: the rule looks right in the file and the game
// quietly falls through to the base filter. This cost an evening once.
func TestUncutRulesDoNotMatchExactly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UncutGemLevel, cfg.UncutSupportLevel = 20, TierHide
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Uncut") && strings.Contains(line, "BaseType ==") {
			t.Errorf("uncut gems must be matched loosely, got: %s", strings.TrimSpace(line))
		}
	}
}

func TestTierSliders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.T5RareTier, cfg.RareJewelTier, cfg.WaystoneTier = 3, 0, 15
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	if blockContaining(t, out, "Show", "Rarity Rare", "UnidentifiedItemTier >= 3") < 0 {
		t.Error("rare equipment should follow its slider")
	}
	// Tier 0 means every rare jewel, so no tier condition belongs in the rule.
	jewels := blockContaining(t, out, "Show", `Class == "Jewels"`, "Rarity Rare")
	if jewels < 0 {
		t.Error("rare jewels should be shown at tier 0")
	}
	if blockContaining(t, out, "Show", `Class == "Jewels"`, "UnidentifiedItemTier") >= 0 {
		t.Error("tier 0 should not write a tier condition")
	}
	if blockContaining(t, out, "Show", `Class == "Waystones"`, "WaystoneTier >= 15") < 0 {
		t.Error("waystones should follow their slider")
	}

	// Off means the rule is not written at all.
	cfg.T5RareTier, cfg.RareJewelTier, cfg.WaystoneTier = TierOff, TierOff, TierOff
	out, _ = GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	if blockContaining(t, out, "Show", "UnidentifiedItemTier") >= 0 {
		t.Error("no tier rule expected when the sliders are off")
	}
	if blockContaining(t, out, "Show", `Class == "Waystones"`) >= 0 {
		t.Error("no waystone rule expected when the slider is off")
	}
	if blockContaining(t, out, "Hide", `Class == "Jewels"`) >= 0 {
		t.Error(`"none" must leave jewels to the base filter, not hide them`)
	}
}

// The leftmost stop hides everything of that kind; the one next to it writes no
// rule at all. Mixing the two up would either bury drops or leak them.
func TestTierHideStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.T5RareTier, cfg.RareJewelTier = TierHide, TierHide
	cfg.WaystoneTier, cfg.UncutGemLevel, cfg.UncutSupportLevel = TierHide, TierHide, TierHide
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	for _, c := range []struct{ what, cond string }{
		{"rare equipment", "Rarity Rare"},
		{"rare jewels", `Class == "Jewels"`},
		{"waystones", `Class == "Waystones"`},
		{"skill gems", `BaseType "Uncut Skill Gem" "Uncut Spirit Gem"`},
		{"support gems", `BaseType "Uncut Support Gem"`},
	} {
		if blockContaining(t, out, "Hide", c.cond) < 0 {
			t.Errorf("%s should be hidden outright", c.what)
		}
	}
	if blockContaining(t, out, "Show", "UnidentifiedItemTier") >= 0 {
		t.Error("nothing should be shown by a hidden slider")
	}

	// And "none" writes nothing for any of them.
	cfg.T5RareTier, cfg.RareJewelTier = TierOff, TierOff
	cfg.WaystoneTier, cfg.UncutGemLevel, cfg.UncutSupportLevel = TierOff, TierOff, TierOff
	out, _ = GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	for _, cond := range []string{`Class == "Waystones"`, `BaseType "Uncut Support Gem"`, `Class == "Jewels"`} {
		if blockContaining(t, out, "Hide", cond) >= 0 {
			t.Errorf("%q: no rule expected at the none stop", cond)
		}
	}
}

// Files written before the hide stop existed used one "off" for both meanings:
// for jewels and support gems it actually hid them, so they must land on hide.
func TestTierMeaningMigration(t *testing.T) {
	legacy := `{"t5_rare_tier": -1, "rare_jewel_tier": -1, "waystone_tier": -1,
	 "uncut_gem_level": -1, "uncut_support_level": -1}`
	p := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := LoadConfig(p)
	if c.RareJewelTier != TierHide || c.UncutSupportLevel != TierHide {
		t.Errorf("jewels and support gems were hidden before: %d / %d", c.RareJewelTier, c.UncutSupportLevel)
	}
	if c.T5RareTier != TierOff || c.WaystoneTier != TierOff || c.UncutGemLevel != TierOff {
		t.Errorf("the others only meant 'no rule': %+v", c)
	}
	// A file that already knows about the hide stop is left alone.
	again := c
	again.RareJewelTier = TierOff
	again.Normalize()
	if again.RareJewelTier != TierOff {
		t.Error("a current file must not be migrated twice")
	}
}

func TestTierMigrationFromToggles(t *testing.T) {
	legacy := `{"t5_rares": false, "t5_jewels_only": false, "high_waystones": true,
	 "high_uncut_gems": true, "uncut_support_gems": true}`
	p := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := LoadConfig(p)
	if c.T5RareTier != TierOff {
		t.Errorf("t5_rares off should switch the slider off, got %d", c.T5RareTier)
	}
	if c.RareJewelTier != 0 {
		t.Errorf(`"T5 only" off used to show every rare jewel, got %d`, c.RareJewelTier)
	}
	if c.WaystoneTier != 14 {
		t.Errorf("waystones were T14+, got %d", c.WaystoneTier)
	}
	if c.UncutGemLevel != MaxUncutGemLevel {
		t.Errorf("uncut gems were level 20, got %d", c.UncutGemLevel)
	}
	if c.UncutSupportLevel != MaxSupportGemLevel {
		t.Errorf("support gems followed the level rule, got %d", c.UncutSupportLevel)
	}
	// The old keys must not come back when the config is written again.
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	for _, gone := range []string{"t5_rares", "high_uncut_gems", "uncut_support_gems"} {
		if strings.Contains(string(data), gone) {
			t.Errorf("legacy key %q written back", gone)
		}
	}
}

func TestTierClamp(t *testing.T) {
	cfg := DefaultConfig()
	cfg.T5RareTier, cfg.UncutGemLevel, cfg.UncutSupportLevel, cfg.WaystoneTier = 99, 0, 99, 99
	cfg.Normalize()
	if cfg.T5RareTier != MaxRareTier || cfg.UncutGemLevel != 1 ||
		cfg.UncutSupportLevel != MaxSupportGemLevel || cfg.WaystoneTier != MaxWaystoneTier {
		t.Errorf("sliders not clamped: %+v", cfg)
	}
}

func TestNeverSinkAndCustomThemes(t *testing.T) {
	ns := map[string]Theme{"apex_stier": {Full: true, BgColor: "255 255 255 255", TextColor: "255 0 0 255",
		Border: "255 0 0 255", Beam: "Red", Icon: "Red", Shape: "Star"}}
	cfg := DefaultConfig()
	cfg.Styles = map[string]string{GroupT5Rare: "ns:apex_stier", GroupExceptionalUnknown: CustomThemeID, GroupUnique: CustomThemeID}
	cfg.CustomStyles = map[string]CustomStyle{
		GroupExceptionalUnknown: {Bg: "#102030", Text: "#ffffff", Border: "#ff0000", Icon: "Green", Shape: "Hexagon"},
		GroupUnique:             {Bg: "red", Text: "#ffffff", Border: "#ff0000"}, // invalid colour
	}
	cfg.Normalize()
	if _, ok := cfg.CustomStyles[GroupUnique]; ok || cfg.Styles[GroupUnique] != "" {
		t.Fatal("invalid custom style kept")
	}
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, ns)

	// A full theme adds a beam to a group that has none and uses its own icon shape.
	if blockContaining(t, out, "UnidentifiedItemTier >= 5", "SetBackgroundColor 255 255 255 255", "PlayEffect Red", "MinimapIcon 2 Red Star") < 0 {
		t.Fatal("NeverSink theme not applied to T5 rares")
	}
	if blockContaining(t, out, "Sockets >= 2", `Class == `, "SetBackgroundColor 16 32 48 255", "SetBorderColor 255 0 0 255", "MinimapIcon 1 Green Hexagon") < 0 {
		t.Fatal("custom theme not applied to unpriced exceptionals")
	}
	for _, blk := range strings.Split(out, "\n\n") {
		if strings.Contains(blk, "16 32 48") && strings.Contains(blk, "PlayEffect") {
			t.Fatal("custom theme without a beam must not add one")
		}
	}
	// A missing NeverSink tag falls back to the group default.
	cfg.Styles[GroupT5Rare] = "ns:gone"
	if p, custom := cfg.Palette(GroupT5Rare, ns); custom || p.BgColor != groupByID[GroupT5Rare].Default.BgColor {
		t.Fatal("unknown NeverSink tag should fall back to the default")
	}
}

func TestUserGroupsOrderAndLook(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ItemGroups = []ItemGroup{
		{ID: "g1", Name: "Loud", Items: []string{"Silk Robe|unique"}, Always: true},
		{ID: "g2", Name: "Quiet", Items: []string{"Orb of Alchemy"}},
		{ID: "g3", Name: "Gone", Items: []string{"Heavy Belt"}, Hide: true},
	}
	cfg.Styles = map[string]string{"user:g1": "neon_green"}
	cfg.Normalize()
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	// An "always" group outranks the valuable styles on the same item.
	loud := blockContaining(t, out, "Show", "Rarity Unique", `"Silk Robe"`, "SetBackgroundColor 20 160 70 255")
	strong := blockContaining(t, out, "Show", "Rarity Unique", `"Silk Robe"`, "PlayEffect Red")
	if loud < 0 {
		t.Fatal("the always group did not use its own colours")
	}
	if strong >= 0 && strong < loud {
		t.Fatalf("an always group must come before the valuable styles (loud=%d strong=%d)", loud, strong)
	}
	// A plain group still waits until after them.
	quiet := blockContaining(t, out, "Show", `"Orb of Alchemy"`, "MinimapIcon 1 Purple Diamond")
	if quiet < 0 || quiet < loud {
		t.Fatalf("a plain group belongs after the always ones (quiet=%d loud=%d)", quiet, loud)
	}
	// Hide groups come first of all.
	hidden := blockContaining(t, out, "Hide", `"Heavy Belt"`)
	if hidden < 0 || hidden > loud {
		t.Fatalf("hide groups must come first (hidden=%d loud=%d)", hidden, loud)
	}
	// Both group names reach the filter as section headings.
	for _, want := range []string{"LOUD", "QUIET", "GONE"} {
		if !strings.Contains(out, want) {
			t.Errorf("section heading %q missing", want)
		}
	}
}

func TestItemGroupNormalize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ItemGroups = []ItemGroup{
		{Name: "  spaced  ", Items: []string{" Chaos Orb ", "", "  "}},
		{ID: "g1"}, // no name
		{ID: "g1", Name: "clash", Hide: true, Always: true}, // duplicate id
	}
	cfg.Styles = map[string]string{"user:ghost": "neon_red", GroupDivine: "neon_red"}
	cfg.CustomStyles = map[string]CustomStyle{"user:ghost": {Bg: "#101010", Text: "#ffffff", Border: "#ffffff"}}
	cfg.Sounds = map[string]string{"user:ghost": "3"}
	cfg.Normalize()

	if n := len(cfg.ItemGroups); n != 3 {
		t.Fatalf("groups kept: %d", n)
	}
	if g := cfg.ItemGroups[0]; g.Name != "spaced" || len(g.Items) != 1 || g.Items[0] != "Chaos Orb" {
		t.Errorf("first group not tidied: %+v", g)
	}
	if cfg.ItemGroups[1].Name == "" {
		t.Error("a nameless group must get a name")
	}
	ids := map[string]bool{}
	for _, g := range cfg.ItemGroups {
		if g.ID == "" || ids[g.ID] {
			t.Errorf("id not unique: %q", g.ID)
		}
		ids[g.ID] = true
	}
	if cfg.ItemGroups[2].Always {
		t.Error("a hidden group has nothing to outrank")
	}
	if cfg.ItemGroups[2].Mode != ItemGroupModeHide || !cfg.ItemGroups[2].Hide {
		t.Errorf("legacy hide group did not gain its mode: %+v", cfg.ItemGroups[2])
	}
	// Styles of a group that no longer exists are cleared, others are kept.
	if _, ok := cfg.Styles["user:ghost"]; ok {
		t.Error("style of a deleted group kept")
	}
	if _, ok := cfg.CustomStyles["user:ghost"]; ok {
		t.Error("custom style of a deleted group kept")
	}
	if _, ok := cfg.Sounds["user:ghost"]; ok {
		t.Error("sound of a deleted group kept")
	}
	if cfg.Styles[GroupDivine] != "neon_red" {
		t.Error("a built-in group style must survive")
	}
}

func TestLegacyListsBecomeGroups(t *testing.T) {
	legacy := `{"whitelist_mid": ["Orb of Alchemy"], "blacklist": ["Scroll of Wisdom"],
	 "styles": {"whitelist_mid": "neon_green"}, "sounds": {"whitelist_mid": "4"}}`
	p := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := LoadConfig(p)
	if len(c.ItemGroups) != 2 {
		t.Fatalf("both lists should become groups: %+v", c.ItemGroups)
	}
	mid, hidden := c.ItemGroups[0], c.ItemGroups[1]
	if mid.Hide || len(mid.Items) != 1 || mid.Items[0] != "Orb of Alchemy" {
		t.Errorf("medium list not carried over: %+v", mid)
	}
	if !hidden.Hide || len(hidden.Items) != 1 {
		t.Errorf("blacklist not carried over: %+v", hidden)
	}
	// The look and sound follow the list into its group.
	if c.Styles[mid.StyleKey()] != "neon_green" || c.Sounds[mid.StyleKey()] != "4" {
		t.Errorf("look not migrated: styles=%v sounds=%v", c.Styles, c.Sounds)
	}
	if _, ok := c.Styles[GroupWhitelistMid]; ok {
		t.Error("the old style key should be gone")
	}
}
