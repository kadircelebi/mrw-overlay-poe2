package invscan

import (
	"image"
	"os"
	"testing"
)

// TestLocalDump reads INVSCAN_SHOT's inventory with INVSCAN_ICONS and lists
// every filled cell as the app would price it (INVSCAN_DUMP set).
func TestLocalDump(t *testing.T) {
	if os.Getenv("INVSCAN_DUMP") == "" {
		t.Skip("INVSCAN_DUMP not set")
	}
	f, err := os.Open(os.Getenv("INVSCAN_SHOT"))
	if err != nil {
		t.Fatal(err)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	pic := FromImage(src, b)
	g := Align(pic, InventoryGuess(b.Dx(), b.Dy()))
	m := NewMatcher(loadIcons(t, os.Getenv("INVSCAN_ICONS")), int(g.Cell+0.5))
	for _, c := range m.ReadGrid(pic, g) {
		name := "(off the list)"
		if c.Icon >= 0 {
			if e, ok := Pick(c.Entries, c.Marks); ok {
				name = e.Name
			} else {
				name = "(unsure) " + c.Entries[0].Name
			}
		}
		t.Logf("r%d c%2d %-44s x%-4d corner %-3d roman %-5v unsure %-5v shape %.2f", c.Row, c.Col, name, c.Stack, c.Corner, c.Roman, c.Unsure, c.Score)
	}
}
