package overlay

import "testing"

// The last area of this session counts; hideouts and towns are skipped, and
// a new game session forgets the old one.
func TestLastArea(t *testing.T) {
	log := `2026/10/03 20:00:00 1 a [INFO Client 1] ***** LOG FILE OPENING *****
2026/10/03 20:01:00 1 a [DEBUG Client 1] Generating level 82 area "MapOld" with seed 1
2026/10/03 22:00:00 1 a [INFO Client 2] ***** LOG FILE OPENING *****
2026/10/03 22:40:02 1 a [DEBUG Client 2] Generating level 65 area "HideoutShrine" with seed 1
2026/10/03 22:40:26 1 a [DEBUG Client 2] Generating level 79 area "MapWaywardIsle" with seed 817132385
2026/10/03 22:45:09 1 a [DEBUG Client 2] Generating level 65 area "HideoutShrine" with seed 1
2026/10/03 22:45:30 1 a [DEBUG Client 2] Generating level 15 area "G1_town" with seed 1
`
	if a := lastArea(log, false); a != (Area{79, "MapWaywardIsle", "817132385"}) {
		t.Errorf("got %+v", a)
	}
	// Where the player is now: the town, hideouts and towns included.
	if a := lastArea(log, true); a != (Area{15, "G1_town", "1"}) {
		t.Errorf("current area %+v", a)
	}
	if a := lastArea("2026/10/03 22:00:00 1 a [INFO Client 2] ***** LOG FILE OPENING *****\n", true); a.Level != 0 {
		t.Errorf("fresh session %+v", a)
	}
}

// The same map id with another seed is another map; hideouts and towns rest.
func TestParseArea(t *testing.T) {
	a, ok := ParseArea(`2026/10/03 22:40:26 1 a [DEBUG Client 2] Generating level 79 area "MapWaywardIsle" with seed 817132385`)
	if !ok || a != (Area{79, "MapWaywardIsle", "817132385"}) || a.Rest() {
		t.Fatalf("%+v %v", a, ok)
	}
	if !a.Same(Area{80, "MapWaywardIsle", "817132385"}) || a.Same(Area{79, "MapWaywardIsle", "1"}) {
		t.Error("Same")
	}
	if h, _ := ParseArea(`x Generating level 65 area "HideoutShrine" with seed 1`); !h.Rest() {
		t.Error("hideout not rest")
	}
	if _, ok := ParseArea("2026/10/03 22:40:26 1 a [INFO Client 2] : You have entered Wayward Isle."); ok {
		t.Error("parsed a non-area line")
	}
}

// The hide shortcut defaults to Alt+H, unless the player already gave Alt+H
// to something else; then it stays off rather than clash.
func TestHideHotkeyDefault(t *testing.T) {
	s := Settings{}
	s.Normalize()
	if s.HideHotkey != "Alt+H" {
		t.Errorf("default = %q", s.HideHotkey)
	}
	s = Settings{Commands: []ChatCommand{{Hotkey: "Alt+H", Text: "/hideout"}}}
	s.Normalize()
	if s.HideHotkey != "" || s.DistinctHotkeys() != nil {
		t.Errorf("clashing default = %q (%v)", s.HideHotkey, s.DistinctHotkeys())
	}
}
