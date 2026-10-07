package overlay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const tierModsByBase = `{
 "Body Armours": {
  "dex_armour,body_armour,armour,default": {
   "bases": ["Metadata/Vest1", "Metadata/UniqueVest"],
   "mods": {
    "prefix": {
     "EvasionPercent": {"LocalEvasion1": 2, "LocalEvasion2": 16, "LocalEvasion3": 75},
     "EvasionHybrid": {"LocalEvasionStun1": 8, "LocalEvasionStun2": 78}
    },
    "suffix": {
     "BleedDuration": {"Bleed1": 30},
     "Strength": {"Strength1": 1, "Strength2": 11}
    }
   }
  }
 },
 "Tablet": {
  "tablet,default": {
   "bases": ["Metadata/Tablet1"],
   "mods": {"prefix": {"RareMonsters": {"TabletRares1": 1}}}
  }
 },
 "One Hand Maces": {
  "mace,default": {
   "bases": ["Metadata/Mace1"],
   "mods": {"prefix": {"AddedPhys": {"Phys1": 1, "Phys2": 20}}}
  }
 },
 "Jewels": {
  "jewel,intjewel,default": {
   "bases": ["Metadata/Sapphire"],
   "mods": {"suffix": {"CriticalStrikeMultiplier": {"JewelCritDamage": 1}}}
  },
  "jewel,int_radius_jewel,radius_jewel,default": {
   "bases": ["Metadata/TimeLostSapphire"],
   "mods": {
    "prefix": {
     "WeaponSpellDamage": {"JewelRadiusSpellDamage": 1},
     "JewelRadiusLargerRadius": {"JewelRadiusMediumSize": 1, "JewelRadiusLargeSize": 1},
     "IncisionChance": {"JewelRadiusIncisionChance": 1}
    },
    "suffix": {
     "CriticalStrikeMultiplier": {"JewelRadiusCriticalDamage": 1},
     "JewelRadiusSmallNodeEffect": {"JewelRadiusSmallNodeEffect": 1}
    }
   }
  }
 }
}`

const tierMods = `{
 "LocalEvasion1": {"name": "Agile", "text": "(15-26)% increased [Evasion|Evasion Rating]", "stats": [{"id": "local_evasion_rating_+%"}]},
 "LocalEvasion2": {"name": "Dancer's", "text": "(27-42)% increased [Evasion|Evasion Rating]", "stats": [{"id": "local_evasion_rating_+%"}]},
 "LocalEvasion3": {"name": "Illusory", "text": "(101-110)% increased [Evasion|Evasion Rating]", "stats": [{"id": "local_evasion_rating_+%"}]},
 "LocalEvasionStun1": {"name": "Mosquito's", "text": "(6-13)% increased [Evasion|Evasion Rating]\n+(6-7) to [StunThreshold|Stun Threshold]", "stats": [{"id": "local_evasion_rating_+%"}, {"id": "local_stun_threshold"}]},
 "LocalEvasionStun2": {"name": "Trickster's", "text": "(39-42)% increased [Evasion|Evasion Rating]\n+(40-50) to [StunThreshold|Stun Threshold]", "stats": [{"id": "local_evasion_rating_+%"}, {"id": "local_stun_threshold"}]},
 "Bleed1": {"name": "of Sealing", "text": "(60-56)% reduced Duration of [Bleeding] on You", "stats": [{"id": "bleed_duration_on_self_+%"}]},
 "Strength1": {"name": "of the Brute", "text": "+(5-8) to [Strength|Strength]", "stats": [{"id": "additional_strength"}]},
 "Strength2": {"name": "of the Wrestler", "text": "+(9-12) to [Strength|Strength]", "stats": [{"id": "additional_strength"}]},
 "TabletRares1": {"name": "Brimming", "text": "Map has (25-35)% increased number of Rare Monsters", "stats": [{"id": "map_rare_monster_num_+%"}]},
 "Phys1": {"name": "Glinting", "text": "Adds (1-2) to (4-5) [Physical|Physical] Damage", "stats": [{"id": "local_minimum_added_physical_damage"}, {"id": "local_maximum_added_physical_damage"}]},
 "Phys2": {"name": "Burnished", "text": "Adds (4-6) to (7-11) [Physical|Physical] Damage", "stats": [{"id": "local_minimum_added_physical_damage"}, {"id": "local_maximum_added_physical_damage"}]},
 "JewelCritDamage": {"name": "of Potency", "text": "(10-15)% increased [CriticalDamageBonus|Critical Damage Bonus]", "stats": [{"id": "base_critical_strike_multiplier_+"}], "spawn_weights": [{"tag": "intjewel", "weight": 1}]},
 "JewelRadiusCriticalDamage": {"name": "of Potency", "text": "(5-10)% increased [CriticalDamageBonus|Critical Damage Bonus]", "stats": [{"id": "base_critical_strike_multiplier_+"}], "spawn_weights": [{"tag": "int_radius_jewel", "weight": 1}, {"tag": "default", "weight": 0}]},
 "JewelRadiusSpellDamage": {"name": "Mystic", "text": "(1-2)% increased [Spell] Damage", "stats": [{"id": "spell_damage_+%"}], "spawn_weights": [{"tag": "int_radius_jewel", "weight": 1}, {"tag": "default", "weight": 0}]},
 "JewelRadiusMediumSize": {"name": "Greater", "text": "Upgrades Radius to Medium", "stats": [{"id": "local_jewel_effect_base_radius"}], "spawn_weights": [{"tag": "radius_jewel", "weight": 1}]},
 "JewelRadiusLargeSize": {"name": "Grand", "text": "Upgrades Radius to Large", "stats": [{"id": "local_jewel_effect_base_radius"}], "spawn_weights": [{"tag": "radius_jewel", "weight": 1}]},
 "JewelRadiusIncisionChance": {"name": "Cutting", "text": "(3-5)% chance for [Attack] [Hit|Hits] to apply [Incision]", "stats": [{"id": "chance_to_inflict_incision_on_attack_hit_%"}], "spawn_weights": [{"tag": "int_radius_jewel", "weight": 1}]},
 "JewelRadiusSmallNodeEffect": {"name": "of Influence", "text": "(15-25)% increased Effect of [SmallPassive|Small Passive] Skills in Radius", "stats": [{"id": "local_jewel_small_passive_in_radius_effect_+%"}], "spawn_weights": [{"tag": "radius_jewel", "weight": 1}, {"tag": "default", "weight": 0}]}
}`

const tierBases = `{
 "Metadata/Vest1": {"name": "Slipstrike Vest", "release_state": "released"},
 "Metadata/UniqueVest": {"name": "Golden Mantle", "release_state": "unique_only"},
 "Metadata/Tablet1": {"name": "Irradiated Tablet", "release_state": "released"},
 "Metadata/Mace1": {"name": "Wooden Club", "release_state": "released"},
 "Metadata/Sapphire": {"name": "Sapphire", "release_state": "released"},
 "Metadata/TimeLostSapphire": {"name": "Time-Lost Sapphire", "release_state": "released"}
}`

func tierCatalog() Catalog {
	return Catalog{Stats: []StatGroup{{ID: "explicit", Entries: []StatEntry{
		{ID: "explicit.stat_global_evasion", Text: "#% increased Evasion Rating", Type: "explicit"},
		{ID: "explicit.stat_local_evasion", Text: "#% increased Evasion Rating (Local)", Type: "explicit"},
		{ID: "explicit.stat_stun", Text: "# to Stun Threshold", Type: "explicit"},
		{ID: "explicit.stat_bleed", Text: "#% increased Duration of Bleeding on You", Type: "explicit"},
		{ID: "explicit.stat_str", Text: "# to Strength", Type: "explicit"},
		{ID: "explicit.stat_rares", Text: "Map has #% increased number of Rare Monsters", Type: "explicit"},
		{ID: "explicit.stat_phys", Text: "Adds # to # Physical Damage (Local)", Type: "explicit"},
		{ID: "implicit.stat_str", Text: "# to Strength", Type: "implicit"},
		{ID: "explicit.stat_crit_damage", Text: "#% increased Critical Damage Bonus", Type: "explicit"},
		{ID: "explicit.stat_spell_damage", Text: "#% increased Spell Damage", Type: "explicit"},
		{ID: "explicit.stat_notable_crit_damage", Text: "Notable Passive Skills in Radius also grant #% increased Critical Damage Bonus", Type: "explicit"},
		{ID: "explicit.stat_small_spell_damage", Text: "Small Passive Skills in Radius also grant #% increased Spell Damage", Type: "explicit"},
		{ID: "explicit.stat_small_effect", Text: "#% increased Effect of Small Passive Skills in Radius", Type: "explicit"},
		{ID: "explicit.stat_radius|1", Text: "Upgrades Radius to Medium", Type: "explicit"},
		{ID: "explicit.stat_radius|2", Text: "Upgrades Radius to Large", Type: "explicit"},
		{ID: "explicit.stat_small_incision", Text: "Small Passive Skills in Radius also grant Attack Hits apply Incision", Type: "explicit"},
	}}}}
}

func buildTestTiers(t *testing.T) *TierData {
	t.Helper()
	data, err := BuildTiers([]byte(tierModsByBase), []byte(tierMods), []byte(tierBases), tierCatalog(), "src")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func tableOf(tables []TierTable, stat string, hybrid bool) *TierTable {
	for i := range tables {
		if tables[i].Stat == stat && tables[i].Hybrid == hybrid {
			return &tables[i]
		}
	}
	return nil
}

func TestTiersAreOrderedBestFirstAndUseTheLocalStat(t *testing.T) {
	tables := buildTestTiers(t).For("Slipstrike Vest", "")
	plain := tableOf(tables, "explicit.stat_local_evasion", false)
	if plain == nil || plain.Affix != "prefix" || len(plain.Tiers) != 3 {
		t.Fatalf("plain evasion table = %+v", plain)
	}
	if got := plain.Tiers[0]; got.Tier != 1 || got.Name != "Illusory" || got.Level != 75 || got.Min != 101 || got.Max != 110 {
		t.Fatalf("T1 = %+v", got)
	}
	if got := plain.Tiers[2]; got.Tier != 3 || got.Min != 15 || got.Max != 26 {
		t.Fatalf("T3 = %+v", got)
	}
}

func TestHybridFamiliesAreTheirOwnTables(t *testing.T) {
	tables := buildTestTiers(t).For("Slipstrike Vest", "")
	hybrid := tableOf(tables, "explicit.stat_local_evasion", true)
	if hybrid == nil || hybrid.Tiers[0].Min != 39 || hybrid.Tiers[0].Name != "Trickster's" || !strings.Contains(hybrid.With, "Stun Threshold") {
		t.Fatalf("hybrid evasion table = %+v", hybrid)
	}
	stun := tableOf(tables, "explicit.stat_stun", true)
	if stun == nil || stun.Tiers[0].Min != 40 || stun.Tiers[0].Max != 50 || !strings.Contains(stun.With, "Evasion") {
		t.Fatalf("hybrid stun table = %+v", stun)
	}
}

func TestReducedRollsAreNegativeIncreasedStats(t *testing.T) {
	bleed := tableOf(buildTestTiers(t).For("Slipstrike Vest", ""), "explicit.stat_bleed", false)
	if bleed == nil || bleed.Tiers[0].Min != -60 || bleed.Tiers[0].Max != -56 {
		t.Fatalf("bleed table = %+v", bleed)
	}
}

func TestAddedDamageTiersUseTheAverage(t *testing.T) {
	phys := tableOf(buildTestTiers(t).For("Wooden Club", ""), "explicit.stat_phys", false)
	// Adds (4-6) to (7-11): the average ranges from 5.5 to 8.5.
	if phys == nil || phys.Tiers[0].Min != 5.5 || phys.Tiers[0].Max != 8.5 || phys.Tiers[1].Min != 2.5 {
		t.Fatalf("phys table = %+v", phys)
	}
}

func TestTiersByClassAndUnreleasedBases(t *testing.T) {
	data := buildTestTiers(t)
	if _, ok := data.Bases["Golden Mantle"]; ok {
		t.Fatal("unique-only base got tiers")
	}
	// A category search knows only the class; the game prints "Tablets".
	rares := tableOf(data.For("", "Tablets"), "explicit.stat_rares", false)
	if rares == nil || rares.Tiers[0].Min != 25 || rares.Tiers[0].Max != 35 {
		t.Fatalf("tablet table = %+v", rares)
	}
	if len(data.For("", "Body Armours")) == 0 || len(data.For("No Such Base", "")) != 0 {
		t.Fatal("class lookup")
	}
}

// A Time-Lost jewel's mods are granted to the passives in its radius: the
// export prints the plain stat, the trade site the granted one.
func TestTimeLostJewelModsAreTheRadiusGrantedStats(t *testing.T) {
	data := buildTestTiers(t)
	timeLost := data.For("Time-Lost Sapphire", "")
	notable := tableOf(timeLost, "explicit.stat_notable_crit_damage", false)
	if notable == nil || notable.Affix != "suffix" || notable.Tiers[0].Min != 5 || notable.Tiers[0].Max != 10 {
		t.Fatalf("notable crit damage table = %+v", notable)
	}
	if small := tableOf(timeLost, "explicit.stat_small_spell_damage", false); small == nil || small.Affix != "prefix" {
		t.Fatalf("small spell damage table = %+v", small)
	}
	// The jewel's own line keeps its plain stat.
	if tableOf(timeLost, "explicit.stat_small_effect", false) == nil {
		t.Fatal("effect of small passives in radius missing")
	}
	for _, plain := range []string{"explicit.stat_crit_damage", "explicit.stat_spell_damage"} {
		if tableOf(timeLost, plain, false) != nil {
			t.Fatalf("Time-Lost Sapphire offers the plain stat %s", plain)
		}
	}
	// Lines the trade site lists without a value are rolled, with no tiers.
	for _, stat := range []string{"explicit.stat_radius|1", "explicit.stat_radius|2", "explicit.stat_small_incision"} {
		if table := tableOf(timeLost, stat, false); table == nil || len(table.Tiers) != 0 {
			t.Fatalf("%s table = %+v", stat, table)
		}
	}
	// A plain Sapphire still rolls the plain stat.
	if crit := tableOf(data.For("Sapphire", ""), "explicit.stat_crit_damage", false); crit == nil || crit.Tiers[0].Min != 10 {
		t.Fatalf("sapphire crit damage table = %+v", crit)
	}
}

func TestTierStoreBuildsOnceAndReusesTheCache(t *testing.T) {
	var gets atomic.Int32
	files := map[string]string{"/mods_by_base.min.json": tierModsByBase, "/mods.min.json": tierMods, "/base_items.min.json": tierBases}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Last-Modified", "Fri, 11 Sep 2026 13:04:23 GMT")
		if r.Method == http.MethodGet {
			gets.Add(1)
			_, _ = w.Write([]byte(body))
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	newStore := func() *TierStore {
		s := NewTierStore(dir, func(context.Context) (Catalog, error) { return tierCatalog(), nil })
		s.baseURL = server.URL + "/"
		return s
	}
	data, err := newStore().Load(context.Background())
	if err != nil || len(data.For("Slipstrike Vest", "")) == 0 {
		t.Fatalf("first load: %v", err)
	}
	if gets.Load() != 3 {
		t.Fatalf("downloads = %d, want 3", gets.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "stat_tiers.json")); err != nil {
		t.Fatal(err)
	}
	// A fresh start reads the file built today instead of downloading.
	if _, err := newStore().Load(context.Background()); err != nil || gets.Load() != 3 {
		t.Fatalf("second load: %v, downloads = %d", err, gets.Load())
	}
}

// TestTiersFromRealExport runs against a downloaded RePoE export and trade
// catalog when POE2_REPOE_DIR names a folder holding mods_by_base.min.json,
// mods.min.json, base_items.min.json and trade_stats.json.
func TestTiersFromRealExport(t *testing.T) {
	dir := os.Getenv("POE2_REPOE_DIR")
	if dir == "" {
		t.Skip("POE2_REPOE_DIR not set")
	}
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	var stats response[StatGroup]
	if err := json.Unmarshal(read("trade_stats.json"), &stats); err != nil {
		t.Fatal(err)
	}
	data, err := BuildTiers(read("mods_by_base.min.json"), read("mods.min.json"), read("base_items.min.json"), Catalog{Stats: stats.Result}, "real")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tables %d, bases %d, classes %d, pools %v", len(data.Tables), len(data.Bases), len(data.Classes), data.PoolNames())
	// Breach's genesis tree modifiers are on no base's list but must be
	// offered: Spirited is T2 of Arcane Surge effect.
	foundSpirited := false
	for _, pool := range data.PoolNames() {
		for _, table := range data.Pool(pool) {
			for _, tier := range table.Tiers {
				if tier.Name == "Spirited" {
					foundSpirited = tier.Tier == 2 && tier.Min == 26 && tier.Max == 32
					t.Logf("%s: %s %+v", pool, table.Stat, table.Tiers)
				}
			}
		}
	}
	if !foundSpirited {
		t.Error("Spirited (Arcane Surge effect, genesis tree) missing from the pools")
	}
	for _, check := range []struct{ base, stat string }{{"Irradiated Tablet", "Rare Monsters"}, {"Heavy Belt", "to Strength"}, {"Slipstrike Vest", "Evasion"}} {
		for _, table := range data.For(check.base, "") {
			var text string
			for _, g := range stats.Result {
				for _, e := range g.Entries {
					if e.ID == table.Stat {
						text = e.Text
					}
				}
			}
			if strings.Contains(text, check.stat) {
				t.Logf("%s | %s | %s hybrid=%v %v", check.base, text, table.Affix, table.Hybrid, table.Tiers)
			}
		}
	}
	// Every Time-Lost jewel line is a radius-granted stat or the jewel's own.
	texts := map[string]string{}
	for _, g := range stats.Result {
		for _, e := range g.Entries {
			texts[e.ID] = e.Text
		}
	}
	for _, base := range []string{"Time-Lost Ruby", "Time-Lost Emerald", "Time-Lost Sapphire", "Time-Lost Diamond"} {
		tables := data.For(base, "")
		granted := 0
		for _, table := range tables {
			text := texts[table.Stat]
			switch {
			case strings.Contains(text, "Passive Skills in Radius also grant"):
				granted++
			case strings.Contains(text, "in Radius"), strings.HasPrefix(text, "Upgrades Radius"):
			default:
				t.Errorf("%s offers the plain stat %q", base, text)
			}
		}
		t.Logf("%s: %d tables, %d radius-granted", base, len(tables), granted)
		if granted == 0 {
			t.Errorf("%s has no radius-granted stats", base)
		}
	}
}
