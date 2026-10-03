package filter

import (
	"strings"
	"testing"
)

var nsExoticBlocks = []string{
	"Show # %D8 $type->exoticbases $tier->pseudocrafts1 !exotics_btier\n\tRarity Normal Magic Rare\n\tBaseType == \"Absent Amulet\" \"Grasping Mail\"\n\tSetFontSize 42",
	"Show # %D8 $type->exoticbases $tier->pseudocrafts2 !exotics_ctier\n\tRarity Normal Magic Rare\n\tBaseType == \"Lament Amulet\" \"Portent Amulet\"",
	"Show # %D6 $type->exoticmods $tier->cmspears !exotics_identifiedmod\n\tMirrored False\n\tCorrupted False\n\tIdentified True\n\tRarity Normal Magic Rare\n\tClass == \"Spears\"\n\tHasExplicitMod >=1 \"Merciless\" \"of the Sniper\"\n\tSetFontSize 42",
}

func TestNeverSinkExotics(t *testing.T) {
	got := NeverSinkExotics(nsExoticBlocks)
	if len(got) != 6 {
		t.Fatalf("entries = %d: %+v", len(got), got)
	}
	absent := got[0]
	if absent.Base != "Absent Amulet" || absent.Level != ExoticHigh || absent.Tier != "pseudocrafts1" || len(absent.Conds) != 1 {
		t.Errorf("absent = %+v", absent)
	}
	if got[2].Base != "Lament Amulet" || got[2].Level != ExoticNormal {
		t.Errorf("lament = %+v", got[2])
	}
	sniper := got[5]
	if sniper.Kind != ExoticMod || sniper.Names[0] != "of the Sniper" || sniper.Classes[0] != "Spears" || len(sniper.Conds) != 4 {
		t.Errorf("sniper = %+v", sniper)
	}
}

// The player's changes: a NeverSink entry switched off, one moved to the
// other level, a modifier of their own added.
func TestExoticGroupRules(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Exotics = nsExoticBlocks
	cfg.Exotic = ExoticSettings{
		Removed: []string{"ns|pseudocrafts1|grasping mail"},
		Levels:  map[string]string{"ns|pseudocrafts2|lament amulet": ExoticHigh},
		Added: []ExoticEntry{
			{Kind: ExoticMod, Classes: []string{"Rings"}, Names: []string{"Spirited"}, Stat: "explicit.stat_x", Label: "increased effect of Arcane Surge"},
			{Kind: ExoticBase, Base: "Prismatic Ring"},
		},
	}
	cfg.Normalize()
	out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)

	high := blockContaining(t, out, "Show", `"Absent Amulet"`, `"Lament Amulet"`)
	if high < 0 {
		t.Errorf("Lament not moved up beside Absent:\n%s", section(out, "7b."))
	}
	if blockContaining(t, out, `"Grasping Mail"`) >= 0 {
		t.Error("switched-off entry written")
	}
	if blockContaining(t, out, "Show", `"Portent Amulet"`) < 0 || blockContaining(t, out, "Show", `"Prismatic Ring"`) < 0 {
		t.Error("normal-level bases missing")
	}
	ring := blockContaining(t, out, "Show", `Class == "Rings"`, `HasExplicitMod >=1 "Spirited"`, "Identified True")
	if ring < 0 {
		t.Errorf("player's modifier missing:\n%s", section(out, "7b."))
	}
	if blockContaining(t, out, `Class == "Spears"`, `HasExplicitMod >=1 "Merciless" "of the Sniper"`) < 0 {
		t.Error("NeverSink's modifier rule missing")
	}

	// Rows for the panel: NeverSink's first, switched-off and moved marked.
	rows := ExoticRows(NeverSinkExotics(cfg.Exotics), cfg.Exotic)
	if len(rows) != 8 {
		t.Fatalf("rows = %d", len(rows))
	}
	for _, r := range rows {
		switch r.Key {
		case "ns|pseudocrafts1|grasping mail":
			if !r.Off {
				t.Error("Grasping Mail not off")
			}
		case "ns|pseudocrafts2|lament amulet":
			if r.Level != ExoticHigh || !r.Changed {
				t.Errorf("Lament row = %+v", r)
			}
		}
	}
	if last := rows[len(rows)-1]; last.Source != "user" || !strings.HasPrefix(last.Key, "user|") {
		t.Errorf("last row = %+v", last)
	}
}

// section returns one section of a written block, for failure messages.
func section(out, title string) string {
	i := strings.Index(out, title)
	if i < 0 {
		return ""
	}
	end := strings.Index(out[i+1:], "#=====")
	if end < 0 {
		return out[i:]
	}
	return out[i : i+1+end]
}

// What the player keeps in the Exotic group wins over their own hide
// groups, with a warning that says so.
func TestExoticBeatsHideGroups(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Exotics = nsExoticBlocks
	cfg.Exotic.Added = []ExoticEntry{{Kind: ExoticBase, Base: "Heavy Belt"}}
	cfg.ItemGroups = []ItemGroup{{ID: "g1", Name: "Junk", Items: []string{"Heavy Belt"}, Mode: ItemGroupModeHide}}
	cfg.Normalize()
	out, st := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
	show := blockContaining(t, out, "Show # $type->exoticbases", `"Heavy Belt"`)
	hide := blockContaining(t, out, "Hide", `"Heavy Belt"`)
	if show < 0 || (hide >= 0 && hide < show) {
		t.Errorf("exotic show %d, group hide %d", show, hide)
	}
	warned := false
	for _, w := range st.Warnings {
		warned = warned || (strings.Contains(w, "Junk") && strings.Contains(w, "Heavy Belt"))
	}
	if !warned {
		t.Errorf("no warning: %v", st.Warnings)
	}
}
