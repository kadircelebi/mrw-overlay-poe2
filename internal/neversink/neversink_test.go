package neversink

import "testing"

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
