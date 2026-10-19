package invscan

import (
	"encoding/json"
	"image"
	"os"
	"testing"
)

// TestLocalSlots reads the marks of the cells listed in INVSCAN_SLOTS (a
// JSON list of [x0, y0, x1, y1]) of INVSCAN_SHOT.
func TestLocalSlots(t *testing.T) {
	path := os.Getenv("INVSCAN_SLOTS")
	if path == "" {
		t.Skip("INVSCAN_SLOTS not set")
	}
	pic, _ := loadShot(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rects [][4]int
	if err := json.Unmarshal(raw, &rects); err != nil {
		t.Fatal(err)
	}
	var scans [][2]cornerScan
	for _, r := range rects {
		st, co := scanMarks(pic, image.Rect(r[0], r[1], r[2], r[3]))
		scans = append(scans, [2]cornerScan{st, co})
	}
	a := anchorsOf(scans)
	t.Logf("anchors %+v", a)
	for i, r := range rects {
		t.Logf("%v %+v", r, marksOf(scans[i][0], scans[i][1], a))
	}
}
