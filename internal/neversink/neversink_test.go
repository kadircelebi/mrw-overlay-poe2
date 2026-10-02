package neversink

import (
	"strings"
	"testing"
)

func TestStyles(t *testing.T) {
	content := `Show # %D9 $type->a $tier->b !apex_stier
	BaseType == "Mirror of Kalandra"
	SetTextColor 255 0 0 255
	SetBackgroundColor 255 255 255 255
	PlayEffect Red
	MinimapIcon 0 Red Star
#	SetBorderColor 1 2 3 255

Show # second rule with the same style !apex_stier
	SetTextColor 9 9 9 255

Hide # hidden rules do not define styles !utility_hidden
	SetBackgroundColor 1 1 1 255

Show # no colours, skipped !currency_supply9
	SetFontSize 30
Show # adjacent block without a blank line !currency_c
	SetBackgroundColor 245 139 87 255
	PlayEffect Purple Temp
`
	got := Styles(content)
	if len(got) != 2 {
		t.Fatalf("got %d styles: %+v", len(got), got)
	}
	a := got[0]
	if a.Tag != "apex_stier" || a.Category != "apex" || a.Name != "stier" || a.Count != 2 ||
		a.Text != "255 0 0 255" || a.Bg != "255 255 255 255" || a.Border != "" ||
		a.Effect != "Red" || a.IconColor != "Red" || a.IconShape != "Star" {
		t.Fatalf("apex style wrong: %+v", a)
	}
	if got[1].Tag != "currency_c" || got[1].Effect != "Purple Temp" {
		t.Fatalf("currency style wrong: %+v", got[1])
	}
}

// A strictness level disables rules by commenting them out. Their base names
// are still valid items, and skipping them left the strictest filter knowing
// the fewest bases — the opposite of what the user asked for.
func TestBaseTypesReadsDisabledRules(t *testing.T) {
	const content = `
Show # $type->ut->rare $tier->gear5c !exotics_btier
	Rarity Rare
	BaseType == "Cavalry Boots" "Champion Helm"
	SetFontSize 40

#Show # %D5 $type->ut->rare $tier->gear4c !exotics_ctier
#	Rarity Rare
#	BaseType == "Warded Helm" "Cassis Helm"
#	SetFontSize 40
`
	bases := BaseTypes(content)
	for _, want := range []string{"Cavalry Boots", "Champion Helm", "Warded Helm", "Cassis Helm"} {
		if got, ok := bases[strings.ToLower(want)]; !ok || got != want {
			t.Errorf("BaseTypes missing %q (got %q, ok=%v)", want, got, ok)
		}
	}
}

// A loose BaseType line is a substring match ("Catalyst" matches every
// catalyst). It names no item, and `BaseType == "Catalyst"` broke the filter.
func TestBaseTypesSkipsLooseMatches(t *testing.T) {
	const content = `
Show # $type->currency->catalysts $tier->restex
	Class == "Stackable Currency"
	BaseType "Catalyst"

Show
	BaseType == "Flesh Catalyst"
`
	bases := BaseTypes(content)
	if _, ok := bases["catalyst"]; ok {
		t.Error("loose fragment \"Catalyst\" read as a base")
	}
	if _, ok := bases["flesh catalyst"]; !ok {
		t.Error("exact base \"Flesh Catalyst\" missing")
	}
}

func TestExceptionalBasesReadsDisabledRules(t *testing.T) {
	const content = `
Show # $type->exotic->exceptional $tier->t1
	BaseType == "Cavalry Boots"

#Show # $type->exotic->exceptional $tier->t2
#	BaseType == "Warded Helm"

Show # $type->ut->rare
	BaseType == "Felt Cap"
`
	got := ExceptionalBases(content)
	if !got["Cavalry Boots"] || !got["Warded Helm"] {
		t.Errorf("exceptional bases incomplete: %v", got)
	}
	if got["Felt Cap"] {
		t.Error("a base outside the exceptional blocks must not be included")
	}
}

// Items ranked by stack size are the ones that drop in stacks; a switched-off
// rule counts too, and a rule without StackSize does not.
func TestStackedBases(t *testing.T) {
	content := `Show # $type->currency->splinter $tier->t1
	StackSize >= 20
	Class == "Stackable Currency"
	BaseType == "Breach Splinter" "Simulacrum Splinter"
	SetFontSize 40

#Show # disabled
#	StackSize >= 150
#	BaseType == "Verisium"

Show
	Class == "Stackable Currency"
	BaseType == "Exalted Orb"
`
	got := StackedBases(content)
	for _, name := range []string{"Breach Splinter", "Simulacrum Splinter", "Verisium"} {
		if !got[name] {
			t.Errorf("%s not read as stacked", name)
		}
	}
	if got["Exalted Orb"] {
		t.Error("Exalted Orb has no StackSize rule")
	}
}
