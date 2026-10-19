package invscan

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLocalInventory reads a real inventory screenshot against downloaded
// icons. Both stay off the repository: set INVSCAN_SHOT (a 3840×2160 PNG)
// and INVSCAN_ICONS (a folder of <key>.png with _index.json).
func TestLocalInventory(t *testing.T) {
	shot, icons := os.Getenv("INVSCAN_SHOT"), os.Getenv("INVSCAN_ICONS")
	if shot == "" || icons == "" {
		t.Skip("INVSCAN_SHOT and INVSCAN_ICONS not set")
	}
	f, err := os.Open(shot)
	if err != nil {
		t.Fatal(err)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	list := loadIcons(t, icons)

	start := time.Now()
	b := src.Bounds()
	pic := FromImage(src, b)
	g := Align(pic, InventoryGuess(b.Dx(), b.Dy()))
	t.Logf("grid %+v (guess %+v)", g, InventoryGuess(b.Dx(), b.Dy()))
	m := NewMatcher(list, int(g.Cell+0.5))
	prepared := time.Since(start)
	cells := m.ReadGrid(pic, g)
	t.Logf("%d filled cells; icons ready in %v, grid read in %v", len(cells), prepared, time.Since(start)-prepared)

	bad := 0
	for _, c := range cells {
		at := [2]int{c.Row, c.Col}
		name, second := "", ""
		if c.Icon >= 0 {
			name = c.Entries[0].Name
		}
		if c.Second >= 0 {
			second = list[c.Second].Entries[0].Name
		}
		want, known := inventoryWant[at]
		mark := ""
		if !known || want != name {
			mark = "  <-- want " + want
		} else if name != "" {
			wantStack := 1
			if s, ok := stackWant[at]; ok {
				wantStack, _ = number(s)
			}
			if wantTier := map[string]int{"II": 2, "III": 3}[cornerWant[at]]; c.Stack != wantStack || c.Corner != wantTier || c.Unsure {
				mark = fmt.Sprintf("  <-- want stack %d tier %d", wantStack, wantTier)
			}
		}
		if mark != "" {
			bad++
		}
		t.Logf("r%d c%2d %-36s x%-3d corner %d shape %.3f color %.3f margin %.3f (next %s)%s", c.Row, c.Col, name, c.Stack, c.Corner, c.Score, c.Color, c.Margin, second, mark)
	}
	if bad > 0 || len(cells) != len(inventoryWant) {
		t.Errorf("%d of %d cells wrong (want %d filled)", bad, len(cells), len(inventoryWant))
	}
}

func loadIcons(t *testing.T, dir string) []Icon {
	raw, err := os.ReadFile(filepath.Join(dir, "_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]struct {
		Items []struct{ ID, Text, Group string } `json:"items"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	var out []Icon
	for key, v := range index {
		f, err := os.Open(filepath.Join(dir, key+".png"))
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		ic := Icon{Key: key, Image: img}
		for _, it := range v.Items {
			ic.Entries = append(ic.Entries, Entry{ID: it.ID, Name: it.Text, Group: it.Group})
		}
		out = append(out, ic)
	}
	return out
}

// inventoryWant is the 2026-10-18 inventory as the player checked it ("" =
// not on the list: tablets, a ring, waystones, and Raven's Reflection, which
// the trade site's exchange data has no icon for). Orb tiers share one icon;
// the first name of the icon is given.
var inventoryWant = map[[2]int]string{
	{0, 0}: "Orb of Alchemy", {0, 1}: "", {0, 2}: "", {0, 3}: "Omen of Greater Exaltation", {0, 4}: "Preserved Jawbone",
	{0, 5}: "Regal Orb", {0, 6}: "Omen of the Blessed", {0, 7}: "Kulemak's Invitation", {0, 8}: "Chaos Orb", {0, 9}: "Expedition Logbook",
	{1, 0}: "Exalted Orb", {1, 1}: "", {1, 2}: "Divine Orb", {1, 3}: "Preserved Cranium", {1, 4}: "Perfect Essence of Abrasion",
	{1, 5}: "Exalted Orb", {1, 6}: "Omen of Sanctification", {1, 7}: "Kulemak's Invitation", {1, 8}: "Simulacrum", {1, 9}: "",
	{2, 0}: "Vaal Orb", {2, 1}: "", {2, 2}: "Exalted Orb", {2, 3}: "Greater Essence of Abrasion", {2, 4}: "Divine Orb",
	{2, 5}: "Omen of the Ancients", {2, 6}: "Perfect Essence of Opulence", {2, 7}: "Perfect Jeweller's Orb", {2, 8}: "Omen of Chaotic Effectiveness",
	{2, 10}: "", {2, 11}: "",
	{3, 0}: "Exalted Orb", {3, 1}: "", {3, 2}: "Ancient Rib", {3, 3}: "Preserved Rib", {3, 4}: "Preserved Collarbone",
	{3, 5}: "Chaos Orb", {3, 6}: "Omen of Sinistral Exaltation", {3, 7}: "Omen of Dextral Crystallisation", {3, 8}: "Omen of Abyssal Echoes",
	{3, 10}: "", {3, 11}: "",
	{4, 0}: "Scroll of Wisdom", {4, 1}: "", {4, 2}: "Perfect Essence of Electricity", {4, 3}: "Orb of Chance", {4, 4}: "Perfect Essence of Grounding",
	{4, 5}: "Ancient Jawbone", {4, 6}: "Omen of Sinistral Crystallisation", {4, 7}: "Omen of Light", {4, 8}: "Chaos Orb",
	{4, 9}: "Potent Liquid Contempt", {4, 10}: "", {4, 11}: "Omen of Amelioration",
}
