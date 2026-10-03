package filtereval

import (
	"os"
	"strings"
	"testing"
)

const sample = `#===============================================================================================================
# NeverSink's Indepth Loot Filter - for Path of Exile 2
#===============================================================================================================

#==============================================================================
# [[DYNAMIC LOOT FILTER]] - poe2-filter
#==============================================================================

#==============================================================================
# KULLANICI GRUBU: ÇÖP
#==============================================================================
Hide
    Class == "Stackable Currency"
    BaseType == "Orb of Transmutation"

#==============================================================================
# 7b. NEVERSINK EXOTIC
#==============================================================================
Show # %D8 $type->exoticbases $tier->pseudocrafts2 !exotics_ctier
	Rarity Normal Magic Rare
	BaseType == "Lament Amulet"
	SetFontSize 40

# [[END DYNAMIC LOOT FILTER]]

#===============================================================================================================
# [[0100]] Gold
#===============================================================================================================
# !! Waypoint c0.start : "Start - Override ALL rules" : "Gold"

Show # $type->decorators->rareeg $tier->ilvl82 !itemproperty_level82
	ItemLevel >= 82
	Rarity Rare
	Class == "Rings" "Amulets"
	Continue

# !! Waypoint c3.exotic.mods : "Exotic Mods" : "Crafting"
Show # %D6 $type->exoticmods $tier->cmspears !exotics_identifiedmod
	Identified True
	Rarity Normal Magic Rare
	Class == "Spears"
	HasExplicitMod >=1 "Merciless" "of the Sniper"
	SetFontSize 42

# !! Waypoint c4.rare : "Rare jewellery" : "Gear"
Show # %D8 $type->ut->rare $tier->j4a !exotics_btier
	UnidentifiedItemTier >= 4
	Rarity Rare
	BaseType == "Prismatic Ring"

#Show # $type->disabled
#	BaseType == "Prismatic Ring"

Show # %D6 $type->rr $tier->jewellery !gear_jewellery1
	Rarity Rare
	Class == "Rings"
	AreaLevel >= 65

Hide # $type->rr $tier->restex !gear_hide
	Rarity Normal Magic Rare
`

func TestParseNamesBlocks(t *testing.T) {
	blocks := Parse(sample)
	if len(blocks) != 7 {
		for _, b := range blocks {
			t.Logf("%d %s %q %s/%s", b.Line, b.Action, b.Section, b.Type, b.Tier)
		}
		t.Fatalf("blocks = %d, want 7 (commented rules skipped)", len(blocks))
	}
	if b := blocks[0]; b.Source != SourceOurs || b.Section != "KULLANICI GRUBU: ÇÖP" || b.Action != Hide {
		t.Errorf("first = %+v", b)
	}
	if b := blocks[1]; b.Source != SourceOurs || b.Section != "7b. NEVERSINK EXOTIC" || b.Type != "exoticbases" || b.Tier != "pseudocrafts2" || b.Style != "exotics_ctier" {
		t.Errorf("exotic = %+v", b)
	}
	if b := blocks[2]; b.Source != SourceNeverSink || b.Section != "Start - Override ALL rules" || !b.Continue {
		t.Errorf("decorator = %+v", b)
	}
	mods := blocks[3].Conds[3]
	if mods.Key != "HasExplicitMod" || mods.CountOp != ">=" || mods.Count != 1 || len(mods.Values) != 2 || mods.Values[1] != "of the Sniper" {
		t.Errorf("HasExplicitMod parsed as %+v", mods)
	}
	if !strings.Contains(blocks[1].Text, "SetFontSize 40") {
		t.Errorf("block text lost its looks: %q", blocks[1].Text)
	}
}

func TestEvaluate(t *testing.T) {
	blocks := Parse(sample)

	// Our hide group decides a currency, certainly.
	res := Evaluate(blocks, Facts{Class: "Stackable Currency", BaseType: "Orb of Transmutation", Rarity: "Normal", StackSize: 3})
	if res.Final == nil || res.Final.Block.Action != Hide || res.Final.Block.Section != "KULLANICI GRUBU: ÇÖP" || len(res.Maybe) != 0 {
		t.Errorf("currency: %+v", res)
	}

	// An identified spear with a listed modifier: NeverSink's exotic mods.
	spear := Facts{Class: "Spears", BaseType: "Flying Spear", Rarity: "Magic", ItemLevel: 80, Identified: true, ExplicitMods: []string{"Merciless", "of Skill"}}
	res = Evaluate(blocks, spear)
	if res.Final == nil || res.Final.Block.Tier != "cmspears" {
		t.Errorf("spear: %+v", res.Final)
	}
	// Unidentified, the game cannot see the modifier.
	spear.Identified = false
	res = Evaluate(blocks, spear)
	if res.Final != nil && res.Final.Block.Tier == "cmspears" {
		t.Error("unidentified spear matched a modifier rule")
	}

	// The Prismatic Ring: the tier rule is only a maybe (its tier is not in
	// the text); the area level is unknown too, so the jewellery rule is a
	// maybe and the catch-all hide decides. The ilvl 82 frame decorates.
	ring := Facts{Class: "Rings", BaseType: "Prismatic Ring", Rarity: "Rare", ItemLevel: 82, Identified: true, ExplicitMods: []string{"Spirited", "Calculating"}}
	res = Evaluate(blocks, ring)
	if res.Final == nil || res.Final.Block.Action != Hide {
		t.Fatalf("ring final = %+v", res.Final)
	}
	if len(res.Maybe) != 2 || res.Maybe[0].Block.Tier != "j4a" || res.Maybe[0].Unknown[0] != "UnidentifiedItemTier >= 4" || res.Maybe[1].Block.Tier != "jewellery" {
		t.Errorf("ring maybes = %+v", res.Maybe)
	}
	if len(res.Decorations) != 1 || res.Decorations[0].Block.Tier != "ilvl82" {
		t.Errorf("ring decorations = %+v", res.Decorations)
	}
	// Knowing the area level settles the jewellery rule.
	ring.AreaLevel = 79
	res = Evaluate(blocks, ring)
	if res.Final == nil || res.Final.Block.Tier != "jewellery" || len(res.Maybe) != 1 {
		t.Errorf("ring in a level 79 area: final %+v maybe %+v", res.Final, res.Maybe)
	}
}

func TestOperators(t *testing.T) {
	f := Facts{Class: "Rings", BaseType: "Prismatic Ring", Rarity: "Magic", ItemLevel: 75, StackSize: 0}
	cases := map[string]Outcome{
		`BaseType "Ring"`:            Yes, // substring
		`BaseType == "Ring"`:         No,
		`BaseType != "Gold Ring"`:    Yes,
		`Rarity <= Magic`:            Yes,
		`Rarity > Magic`:             No,
		`Rarity Normal Rare`:         No,
		`ItemLevel 75`:               Yes,
		`ItemLevel >= 76`:            No,
		`StackSize 1`:                Yes, // a single item is a stack of one
		`Corrupted False`:            Yes,
		`UnidentifiedItemTier >= 1`:  Unknown,
		`HasExplicitMod "Merciless"`: No,
	}
	for line, want := range cases {
		key, rest, _ := strings.Cut(line, " ")
		if got := evalCond(parseCond(key, rest, line), f); got != want {
			t.Errorf("%s: got %v, want %v", line, got, want)
		}
	}
}

// With POE2_FILTER_FILE set to a written filter, a few real items are
// checked against it (the file is not part of the repository).
func TestRealFilter(t *testing.T) {
	path := os.Getenv("POE2_FILTER_FILE")
	if path == "" {
		t.Skip("POE2_FILTER_FILE not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blocks := Parse(string(raw))
	t.Logf("%d blocks", len(blocks))
	for name, f := range map[string]Facts{
		"lament":  {Class: "Amulets", BaseType: "Lament Amulet", Rarity: "Normal", ItemLevel: 82, Identified: true},
		"ring":    {Class: "Rings", BaseType: "Prismatic Ring", Rarity: "Rare", ItemLevel: 81, Identified: true, ExplicitMods: []string{"Spirited", "Crackling", "Calculating", "of Sortilege"}, AreaLevel: 79},
		"divine":  {Class: "Stackable Currency", BaseType: "Divine Orb", Rarity: "Normal", StackSize: 1, AreaLevel: 79},
		"transmu": {Class: "Stackable Currency", BaseType: "Orb of Transmutation", Rarity: "Normal", StackSize: 1, AreaLevel: 79},
	} {
		res := Evaluate(blocks, f)
		if res.Final == nil {
			t.Logf("%s: no rule", name)
			continue
		}
		t.Logf("%s: %s %s line %d %q %s/%s", name, res.Final.Block.Action, res.Final.Block.Source, res.Final.Block.Line, res.Final.Block.Section, res.Final.Block.Type, res.Final.Block.Tier)
		for _, m := range res.Maybe {
			t.Logf("   maybe line %d %q %s/%s unknown %v", m.Block.Line, m.Block.Section, m.Block.Type, m.Block.Tier, m.Unknown)
		}
	}
}
