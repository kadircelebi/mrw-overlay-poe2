package invscan

import (
	"image"
	"os"
	"testing"
	"time"
)

// TestLocalCacheSpeed reads INVSCAN_SHOT twice through a GridCache.
func TestLocalCacheSpeed(t *testing.T) {
	shot, icons := os.Getenv("INVSCAN_SHOT"), os.Getenv("INVSCAN_ICONS")
	if shot == "" || icons == "" || os.Getenv("INVSCAN_CACHE") == "" {
		t.Skip()
	}
	f, _ := os.Open(shot)
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	pic := FromImage(src, b)
	g := Align(pic, InventoryGuess(b.Dx(), b.Dy()))
	m := NewMatcher(loadIcons(t, icons), int(g.Cell+0.5))
	var c GridCache
	for i := 0; i < 3; i++ {
		start := time.Now()
		cells := m.ReadGridCached(pic, g, &c)
		t.Logf("read %d: %d cells in %v", i, len(cells), time.Since(start))
	}
}
