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
	if level, area := lastArea(log); level != 79 || area != "MapWaywardIsle" {
		t.Errorf("got %d %q", level, area)
	}
	if level, _ := lastArea("2026/10/03 22:00:00 1 a [INFO Client 2] ***** LOG FILE OPENING *****\n"); level != 0 {
		t.Errorf("fresh session level %d", level)
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
