package invscan

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

func solid(w, h int, r, g, b float32) Img {
	m := newImg(w, h)
	for i := 0; i < w*h; i++ {
		m.Pix[i*3], m.Pix[i*3+1], m.Pix[i*3+2] = r, g, b
	}
	return m
}

func noise(rng *rand.Rand, w, h int) Img {
	m := newImg(w, h)
	for i := range m.Pix {
		m.Pix[i] = float32(rng.Intn(256))
	}
	return m
}

func TestResizeAveragesAndKeepsFlatColour(t *testing.T) {
	m := solid(64, 64, 10, 20, 30).Resize(100, 100)
	for i := 0; i < len(m.Pix); i += 3 {
		if math.Abs(float64(m.Pix[i]-10)) > 1e-3 || math.Abs(float64(m.Pix[i+2]-30)) > 1e-3 {
			t.Fatalf("enlarging a flat picture changed it: %v", m.Pix[i:i+3])
		}
	}
	// A black and a white column shrink to one grey pixel.
	two := newImg(2, 1)
	two.Pix = []float32{0, 0, 0, 255, 255, 255}
	if got := two.Resize(1, 1).Pix[0]; math.Abs(float64(got)-127.5) > 1e-3 {
		t.Errorf("shrinking averages: got %v", got)
	}
}

func TestNCCFindsTheTemplateAtAnyBrightness(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pic := noise(rng, 60, 50)
	tpl := pic.crop(image.Rect(17, 9, 37, 29))
	// Darker and with less contrast: the same picture to NCC.
	for i := range tpl.Pix {
		tpl.Pix[i] = tpl.Pix[i]*0.6 + 20
	}
	s, at := NCC(pic, tpl)
	if at != image.Pt(17, 9) || s < 0.999 {
		t.Errorf("found %.4f at %v, want ~1 at (17,9)", s, at)
	}
	// Masked-out pixels do not count: scribbling on them changes nothing.
	tpl.Mask = make([]float32, tpl.W*tpl.H)
	for i := range tpl.Mask {
		tpl.Mask[i] = 1
	}
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			tpl.Mask[y*tpl.W+x] = 0
			copy(tpl.Pix[(y*tpl.W+x)*3:], []float32{255, 0, 255})
		}
	}
	if s2, at2 := NCC(pic, tpl); at2 != at || math.Abs(s2-s) > 1e-6 {
		t.Errorf("masked corner changed the match: %.4f at %v", s2, at2)
	}
	if s, _ := nccIn(pic, tpl, image.Rect(0, 0, 5, 5)); s > 0.5 {
		t.Errorf("a search away from the template still matched: %.3f", s)
	}
}

// gridPicture draws cols×rows cells of side cell from (x, y) on a grey
// ground, with dark lines of width line between and around them.
func gridPicture(w, h int, x, y, cell float64, cols, rows, line int) Img {
	pic := solid(w, h, 70, 70, 70)
	set := func(px, py int) {
		if px >= 0 && py >= 0 && px < w && py < h {
			copy(pic.Pix[(py*w+px)*3:], []float32{5, 5, 5})
		}
	}
	for k := 0; k <= cols; k++ {
		c := int(math.Round(x + float64(k)*cell))
		for d := -line / 2; d <= line/2; d++ {
			for py := int(y); py < int(y+float64(rows)*cell); py++ {
				set(c+d, py)
			}
		}
	}
	for k := 0; k <= rows; k++ {
		r := int(math.Round(y + float64(k)*cell))
		for d := -line / 2; d <= line/2; d++ {
			for px := int(x); px < int(x+float64(cols)*cell); px++ {
				set(px, r+d)
			}
		}
	}
	return pic
}

func TestAlignFindsTheGridNearTheGuess(t *testing.T) {
	want := Grid{X: 107, Y: 63, Cell: 52.6, Cols: 12, Rows: 5}
	pic := gridPicture(800, 400, want.X, want.Y, want.Cell, want.Cols, want.Rows, 4)
	guess := want
	guess.X, guess.Y, guess.Cell = want.X+7, want.Y-6, 51.5
	got := Align(pic, guess)
	if math.Abs(got.X-want.X) > 1 || math.Abs(got.Y-want.Y) > 1 || math.Abs(got.Cell-want.Cell) > 0.3 {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestInventoryGuessFollowsTheHeightAndRightEdge(t *testing.T) {
	g := InventoryGuess(3840, 2160)
	if math.Abs(g.X-2544) > 0.5 || math.Abs(g.Y-1181) > 0.5 || math.Abs(g.Cell-105.3) > 0.05 {
		t.Errorf("4K: %+v", g)
	}
	// Ultrawide at 1440: the same panel, half again as tall, at the right.
	w := InventoryGuess(3440, 1440)
	if math.Abs(w.Cell-105.3*1440/2160) > 0.05 || math.Abs((3440-w.X)-(3840-2544)*1440/2160.0) > 0.5 {
		t.Errorf("ultrawide: %+v", w)
	}
}

func TestEmptyTellsGreyFromBlueGround(t *testing.T) {
	if !Empty(solid(50, 50, 30, 30, 32)) {
		t.Error("a dark grey cell is empty")
	}
	if Empty(solid(50, 50, 20, 25, 70)) {
		t.Error("a blue cell holds an item")
	}
}

// blob draws a test icon: a coloured disc on transparency, with a stripe so
// that shapes differ too.
func blob(n int, c color.NRGBA, stripe int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	r := float64(n) * 0.42
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)-float64(n)/2, float64(y)-float64(n)/2
			if dx*dx+dy*dy > r*r {
				continue
			}
			p := c
			if (x+y*stripe)%13 < 5 {
				p = color.NRGBA{c.R / 3, c.G / 3, c.B / 3, 255}
			}
			img.SetNRGBA(x, y, p)
		}
	}
	return img
}

// inCell draws icon into a cell of side cell on the game's blue ground,
// scaled to frac of it and shifted by (dx, dy), with a "number" scribbled
// in the top-left corner.
func inCell(icon image.Image, cell int, frac float64, dx, dy int) Img {
	out := solid(cell, cell, 18, 22, 64)
	src := fromIcon(icon)
	n := int(float64(cell)*frac + 0.5)
	s := src.Resize(n, n)
	ox, oy := (cell-n)/2+dx, (cell-n)/2+dy
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if s.Mask[y*n+x] == 0 || ox+x < 0 || oy+y < 0 || ox+x >= cell || oy+y >= cell {
				continue
			}
			copy(out.Pix[((oy+y)*cell+ox+x)*3:], s.Pix[(y*n+x)*3:(y*n+x)*3+3])
		}
	}
	for y := 2; y < cell/5; y++ {
		for x := 2; x < cell/3; x++ {
			if (x+y)%3 == 0 {
				copy(out.Pix[(y*cell+x)*3:], []float32{240, 240, 240})
			}
		}
	}
	return out
}

func TestMatcherTellsIconsApartAndRejectsStrangers(t *testing.T) {
	icons := []Icon{
		{Key: "red", Entries: []Entry{{Name: "Red"}}, Image: blob(64, color.NRGBA{220, 40, 30, 255}, 1)},
		{Key: "orange", Entries: []Entry{{Name: "Orange"}}, Image: blob(64, color.NRGBA{230, 140, 20, 255}, 1)},
		{Key: "green", Entries: []Entry{{Name: "Green"}}, Image: blob(64, color.NRGBA{40, 200, 60, 255}, 3)},
		{Key: "blue", Entries: []Entry{{Name: "Blue"}}, Image: blob(64, color.NRGBA{60, 90, 230, 255}, 7)},
	}
	const cell = 105
	m := NewMatcher(icons, cell)
	pad := Pad(cell)
	for i, ic := range icons {
		for _, d := range [][2]int{{0, 0}, {3, -2}, {-4, 4}} {
			c := inCell(ic.Image, cell+2*pad, float64(cell)*0.95/float64(cell+2*pad), d[0], d[1])
			r := m.Classify(c)
			if r.Icon != i || r.Score < 0.9 {
				t.Errorf("%s shifted %v: got icon %d score %.3f", ic.Key, d, r.Icon, r.Score)
			}
		}
	}
	rng := rand.New(rand.NewSource(2))
	if r := m.Classify(noise(rng, cell+2*pad, cell+2*pad)); r.Icon != -1 {
		t.Errorf("noise matched icon %d (%.3f)", r.Icon, r.Score)
	}
	got := m.ClassifyAll([]Img{
		inCell(icons[2].Image, cell+2*pad, 0.85, 0, 0),
		inCell(icons[0].Image, cell+2*pad, 0.85, 0, 0),
	})
	if got[0].Icon != 2 || got[1].Icon != 0 {
		t.Errorf("ClassifyAll kept the order wrong: %d, %d", got[0].Icon, got[1].Icon)
	}
}

// drawFigures paints font's first picture of each figure of s side by side
// into m from (x, y), on a cell of figCell pixels.
func drawFigures(m Img, s string, x, y int) {
	for _, r := range s {
		for _, f := range font {
			if f.R != r {
				continue
			}
			for py := 0; py < f.H; py++ {
				for px := 0; px < f.W; px++ {
					v := float32(f.Pix[py*f.W+px])
					copy(m.Pix[((y+py)*m.W+x+px)*3:], []float32{v, v, v})
				}
			}
			x += f.W - 1 // about the game's spacing: outlines nearly touch
			break
		}
	}
}

func TestReadMarksReadsTheFontsOwnFigures(t *testing.T) {
	for _, tc := range []struct {
		stack, corner string
		want          Marks
	}{
		{"39", "", Marks{Stack: 39}},
		{"1000", "", Marks{Stack: 1000}},
		{"976", "", Marks{Stack: 976}},
		{"1", "III", Marks{Stack: 1, Corner: 3, Roman: true}},
		{"20", "II", Marks{Stack: 20, Corner: 2, Roman: true}},
		{"", "9", Marks{Stack: 1, Corner: 9}},
		{"", "", Marks{Stack: 1}},
	} {
		// A blue cell of figCell pixels with a margin round it, as the grid
		// gives it.
		pic := solid(figCell+20, figCell+20, 18, 22, 64)
		cell := image.Rect(10, 10, 10+figCell, 10+figCell)
		drawFigures(pic, tc.stack, cell.Min.X+16, cell.Min.Y+12)
		if tc.corner != "" {
			w := 0
			for _, r := range tc.corner {
				for _, f := range font {
					if f.R == r {
						w += f.W - 1
						break
					}
				}
			}
			drawFigures(pic, tc.corner, cell.Max.X-6-w, cell.Max.Y-36)
		}
		if got := ReadMarks(pic, cell); got != tc.want {
			t.Errorf("%q/%q: got %+v, want %+v", tc.stack, tc.corner, got, tc.want)
		}
	}
}

func TestRomanNumerals(t *testing.T) {
	for s, want := range map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "IX": 9, "XIV": 14, "XIX": 19, "XX": 20} {
		if n, ok := roman(s); !ok || n != want {
			t.Errorf("roman(%q) = %d %v, want %d", s, n, ok, want)
		}
	}
	for _, s := range []string{"", "12", "I2"} {
		if _, ok := roman(s); ok {
			t.Errorf("roman(%q) read a number", s)
		}
	}
}

func TestPickByTheCornersNumber(t *testing.T) {
	orbs := []Entry{{Name: "Exalted Orb"}, {Name: "Greater Exalted Orb"}, {Name: "Perfect Exalted Orb"}}
	flux := []Entry{{Name: "Thaumaturgic Flux (Level 2)"}, {Name: "Thaumaturgic Flux (Level 19)"}, {Name: "Thaumaturgic Flux (Level 20)"}}
	runes := []Entry{{Name: "Legacy of Bramblejack"}, {Name: "Legacy of Blackbraid"}}
	for _, tc := range []struct {
		entries []Entry
		marks   Marks
		want    string
	}{
		{orbs, Marks{Stack: 20}, "Exalted Orb"},
		{orbs, Marks{Stack: 1, Corner: 2, Roman: true}, "Greater Exalted Orb"},
		{orbs, Marks{Stack: 1, Corner: 3, Roman: true}, "Perfect Exalted Orb"},
		{orbs, Marks{Stack: 1, Corner: 3}, ""},                            // an arabic 3 is no tier
		{orbs, Marks{Stack: 1, Corner: 3, Roman: true, Unsure: true}, ""}, // maybe more figures
		{flux, Marks{Stack: 1, Corner: 19, Roman: true}, "Thaumaturgic Flux (Level 19)"},
		{flux, Marks{Stack: 1, Corner: 7, Roman: true}, ""}, // not a listed level
		{flux, Marks{Stack: 1}, ""},                         // no level read
		{runes, Marks{Stack: 1}, ""},                        // unrelated items, one icon
		{[]Entry{{Name: "Divine Orb"}}, Marks{Stack: 4, Unsure: true}, "Divine Orb"},
	} {
		e, ok := Pick(tc.entries, tc.marks)
		if got := map[bool]string{true: e.Name}[ok]; got != tc.want {
			t.Errorf("%s %+v: got %q (%v), want %q", tc.entries[0].Name, tc.marks, e.Name, ok, tc.want)
		}
	}
}

// drawVee paints a white V with a dark outline, figure-sized, its bottom
// at y: a figure font does not know (yet).
func drawVee(m Img, x, y, w, h int) {
	for py := y - h - 3; py < y+3; py++ {
		for px := x - 3; px < x+w+3; px++ {
			copy(m.Pix[(py*m.W+px)*3:], []float32{10, 10, 10})
		}
	}
	for py := y - h; py < y; py++ {
		t := float64(py-(y-h)) / float64(h)
		for _, cx := range []int{x + int(t*float64(w-3)/2), x + w - 3 - int(t*float64(w-3)/2)} {
			for d := 0; d < 3; d++ {
				copy(m.Pix[(py*m.W+cx+d)*3:], []float32{245, 245, 245})
			}
		}
	}
}

func TestReadMarksIsUnsureOfAnUnknownFigure(t *testing.T) {
	pic := solid(figCell+20, figCell+20, 18, 22, 64)
	cell := image.Rect(10, 10, 10+figCell, 10+figCell)
	// "I" at the corner, an unknown figure left of it on its line: a level
	// such as VI must not be read as I.
	var bar figure
	for _, f := range font {
		if f.R == 'I' {
			bar = f
			break
		}
	}
	x := cell.Max.X - 6 - bar.W
	drawFigures(pic, "I", x, cell.Max.Y-36)
	drawVee(pic, x-14, cell.Max.Y-36+bar.H-figOutline, 12, bar.H-2*figOutline)
	if got := ReadMarks(pic, cell); !got.Unsure {
		t.Errorf("an unknown figure beside the number: %+v, want unsure", got)
	}
}

func TestShownTellsAGridFromTheWorld(t *testing.T) {
	g := Grid{X: 20, Y: 10, Cell: 50, Cols: 12, Rows: 5}
	pic := solid(700, 300, 6, 6, 7) // empty cells
	for c := 1; c < g.Cols; c++ {
		x := int(g.X) + c*int(g.Cell)
		for y := int(g.Y); y < int(g.Y)+g.Rows*int(g.Cell); y++ {
			copy(pic.Pix[(y*pic.W+x)*3:], []float32{60, 58, 55})
		}
	}
	for c := 0; c < 5; c++ {
		r := g.Rect(c, 1)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				copy(pic.Pix[(y*pic.W+x)*3:], []float32{18, 22, 64})
			}
		}
	}
	if !Shown(pic, g) {
		t.Error("a grid of empty and blue cells is shown")
	}
	if Shown(solid(700, 300, 0, 0, 0), g) {
		t.Error("a black screen taken for an empty grid")
	}
	rng := rand.New(rand.NewSource(3))
	if Shown(noise(rng, 700, 300), g) {
		t.Error("noise taken for a grid")
	}
}

func TestReadGridCachedRereadsOnlyChangedCells(t *testing.T) {
	icons := []Icon{
		{Key: "red", Entries: []Entry{{Name: "Red"}}, Image: blob(64, color.NRGBA{220, 40, 30, 255}, 1)},
		{Key: "green", Entries: []Entry{{Name: "Green"}}, Image: blob(64, color.NRGBA{40, 200, 60, 255}, 3)},
	}
	const cell = 105
	g := Grid{X: 20, Y: 20, Cell: cell, Cols: 4, Rows: 1}
	pic := solid(4*cell+40, cell+40, 30, 30, 32)
	put := func(col int, icon image.Image) {
		c := inCell(icon, cell, 0.95, 0, 0)
		r := g.Rect(col, 0)
		for y := 0; y < cell; y++ {
			copy(pic.Pix[((r.Min.Y+y)*pic.W+r.Min.X)*3:], c.Pix[y*cell*3:(y+1)*cell*3])
		}
	}
	put(0, icons[0].Image)
	put(2, icons[1].Image)
	m := NewMatcher(icons, cell)
	var cache GridCache
	first := m.ReadGridCached(pic, g, &cache)
	if len(first) != 2 || first[0].Icon != 0 || first[1].Icon != 1 {
		t.Fatalf("first read %+v", first)
	}
	// The same picture again: every cell from the cache, the same answers.
	again := m.ReadGridCached(pic, g, &cache)
	if len(again) != 2 || again[0].Icon != 0 || again[1].Icon != 1 {
		t.Fatalf("cached read %+v", again)
	}
	// An item put into an empty cell is read; a cell that did not change
	// keeps its answer.
	put(3, icons[0].Image)
	third := m.ReadGridCached(pic, g, &cache)
	if len(third) != 3 || third[2].Col != 3 || third[2].Icon != 0 {
		t.Fatalf("after a change %+v", third)
	}
	if direct := m.ReadGrid(pic, g); len(direct) != len(third) {
		t.Fatalf("cache and a full read disagree: %d vs %d cells", len(third), len(direct))
	}
}

func TestBareTellsAnItemsOtherCell(t *testing.T) {
	// A cell of a bigger item: the blue ground and its number in the corner.
	pic := solid(figCell+20, figCell+20, 18, 22, 64)
	cell := image.Rect(10, 10, 10+figCell, 10+figCell)
	drawFigures(pic, "10", cell.Max.X-30, cell.Max.Y-36)
	if !Bare(pic, cell) {
		t.Error("a cell with only a corner number is not bare")
	}
	// An item's own cell: its picture over the middle.
	icon := blob(64, color.NRGBA{200, 140, 60, 255}, 1)
	full := inCell(icon, figCell+20, 0.85, 0, 0)
	if Bare(full, cell) {
		t.Error("a cell with an icon taken for bare")
	}
}
