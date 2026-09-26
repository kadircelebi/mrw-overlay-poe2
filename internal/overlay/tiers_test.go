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
 "Phys2": {"name": "Burnished", "text": "Adds (4-6) to (7-11) [Physical|Physical] Damage", "stats": [{"id": "local_minimum_added_physical_damage"}, {"id": "local_maximum_added_physical_damage"}]}
}`

const tierBases = `{
 "Metadata/Vest1": {"name": "Slipstrike Vest", "release_state": "released"},
 "Metadata/UniqueVest": {"name": "Golden Mantle", "release_state": "unique_only"},
 "Metadata/Tablet1": {"name": "Irradiated Tablet", "release_state": "released"},
 "Metadata/Mace1": {"name": "Wooden Club", "release_state": "released"}
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
	t.Logf("tables %d, bases %d, classes %d", len(data.Tables), len(data.Bases), len(data.Classes))
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
}
