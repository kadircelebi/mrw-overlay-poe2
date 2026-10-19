package invscan

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalCells reads the figures of the cell pictures the app keeps for
// cells it was not sure of (INVSCAN_CELLS, a folder of debug/invscan PNGs:
// each the cell cut with Pad on every side). Their names carry what the
// read then made of them.
func TestLocalCells(t *testing.T) {
	dir := os.Getenv("INVSCAN_CELLS")
	if dir == "" {
		t.Skip("INVSCAN_CELLS not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	type cut struct {
		name  string
		pic   Img
		cell  image.Rectangle
		scans [2]cornerScan
	}
	var cuts []cut
	defer func() {
		// Read together, as one grid's cells: with the anchors they give.
		var scans [][2]cornerScan
		for _, c := range cuts {
			scans = append(scans, c.scans)
		}
		a := anchorsOf(scans)
		t.Logf("anchors %+v", a)
		for _, c := range cuts {
			if _, _, run := c.scans[0].read(false, noAnchor); len(run) > 0 {
				t.Logf("first figure %q at x %d y %d (score %.2f), sub %dx%d", run[0].r, run[0].rect.Min.X, run[0].rect.Min.Y, run[0].score, c.scans[0].sub.W, c.scans[0].sub.H)
			}
			m := marksOf(c.scans[0], c.scans[1], a)
			t.Logf("anchored %-60s → stack %5d corner %3d unsure %v", c.name, m.Stack, m.Corner, m.Unsure)
		}
	}()
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		src, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		pic := FromImage(src, src.Bounds())
		// The cell size whose cut is this wide.
		cell := 1
		for c := 1; c <= pic.W; c++ {
			if c+2*Pad(float64(c)) <= pic.W {
				cell = c
			}
		}
		pad := Pad(float64(cell))
		r := image.Rect(pad, pad, pad+cell, pad+cell)
		m := ReadMarks(pic, r)
		st, co := scanMarks(pic, r)
		cuts = append(cuts, cut{strings.TrimSuffix(filepath.Base(path), ".png"), pic, r, [2]cornerScan{st, co}})
		t.Logf("%-60s cell %3d → stack %5d corner %3d roman %v unsure %v", strings.TrimSuffix(filepath.Base(path), ".png"), cell, m.Stack, m.Corner, m.Roman, m.Unsure)
	}
}
