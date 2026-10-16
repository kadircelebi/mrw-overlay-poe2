package trade

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"
)

// The embedded table is a copy of the craft data's levels; a craft data
// refresh must bring it along (build/craft/build_data.py writes both).
func TestBaseLevelsMatchCraftData(t *testing.T) {
	raw, err := os.ReadFile("../../frontend/public/craft/data/bases.json")
	if err != nil {
		t.Fatal(err)
	}
	var craft struct {
		Pages map[string][]struct {
			Name  string `json:"name"`
			Level int    `json:"level"`
			Req   struct {
				Level int `json:"level"`
			} `json:"req"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &craft); err != nil {
		t.Fatal(err)
	}
	// Two bases can share a name (Ornate Buckler 52 and 65); trade searches by
	// name, so the higher level counts.
	want := map[string]int{}
	for _, entries := range craft.Pages {
		for _, e := range entries {
			lv := e.Req.Level
			if lv == 0 {
				lv = e.Level
			}
			want[e.Name] = max(want[e.Name], lv)
		}
	}
	for name, lv := range want {
		if got, ok := BaseLevel(name); !ok || got != lv {
			t.Fatalf("%s: table %d (%v), craft data %d; run build/craft/build_data.py", name, got, ok, lv)
		}
	}
	if len(want) != len(baseLevels()) {
		t.Fatalf("table has %d bases, craft data %d", len(baseLevels()), len(want))
	}
	if lv, _ := BaseLevel("Ornate Buckler"); lv != 65 {
		t.Errorf("Ornate Buckler level %d, want the higher 65", lv)
	}
	if lv, _ := BaseLevel("Vile Robe"); lv != 65 {
		t.Errorf("Vile Robe level %d, want 65", lv)
	}
}

// Bases from level 65 up are scanned, low ones only when NeverSink lists
// them, and a base the level table does not know (one that does not drop)
// not at all. NeverSink's valuable bases go first; its chance bases do not.
func TestBuildCandidatesSkipsLowLevelBases(t *testing.T) {
	ex := Exceptions{
		Valuable: map[string]bool{"Withered Wand": true, "Vile Robe": true},
		Chance:   map[string]bool{"Stocky Mitts": true},
	}
	bases := []string{"Vile Robe", "Warlord Cuirass", "Withered Wand", "Stocky Mitts", "Leather Vest", "Not A Real Base"}
	got := map[string]int{}
	for _, c := range BuildCandidates(bases, ex) {
		got[c.Base] = c.Priority
	}
	want := map[string]int{"Vile Robe": 0, "Warlord Cuirass": 1, "Withered Wand": 0, "Stocky Mitts": 1}
	if len(got) != len(want) {
		t.Fatalf("candidates %v, want %v", got, want)
	}
	for b, p := range want {
		if q, ok := got[b]; !ok || q != p {
			t.Errorf("%s: priority %d (%v), want %d", b, q, ok, p)
		}
	}
	if low := LowLevelBases(bases, ex); !slices.Equal(low, []string{"Leather Vest"}) {
		t.Errorf("low-level bases %v, want [Leather Vest]", low)
	}
}

// Dropping a base from the scan list drops its results, and an import does
// not bring them back.
func TestScannerForgetsBasesNoLongerScanned(t *testing.T) {
	now := time.Now().UTC()
	s := newTestScanner(t, "Forbidden Rites")
	s.st.Keys["Vile Robe|sockets"] = priced("Vile Robe", 50, now)
	s.st.Keys["Leather Vest|sockets"] = priced("Leather Vest", 30, now)
	s.SetCandidates([]Candidate{{Base: "Vile Robe"}})
	if _, ok := s.st.Keys["Leather Vest|sockets"]; ok {
		t.Fatal("a base off the list kept its price")
	}
	if _, ok := s.st.Keys["Vile Robe|sockets"]; !ok {
		t.Fatal("a listed base lost its price")
	}

	giver := newTestScanner(t, "Forbidden Rites")
	giver.st.Keys["Leather Vest|quality"] = priced("Leather Vest", 30, now)
	data, err := giver.Export()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Import(data); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.st.Keys["Leather Vest|quality"]; ok {
		t.Error("an import brought back a base no longer scanned")
	}
}
