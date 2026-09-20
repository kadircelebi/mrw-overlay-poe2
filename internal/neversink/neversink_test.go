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
// the fewest bases — the opposite of what a stricter level should do.
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
