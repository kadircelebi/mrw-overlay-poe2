package invscan

import (
	"image"
	"math"
	"sort"
)

// The game writes a stack's size in the top-left corner of its cell and a
// gem's level, or a Greater/Perfect orb's II/III, in the bottom-right one,
// in white serif figures with a thin dark outline.

// Corners of a cell (shares of its side) where the figures are written.
var (
	stackCorner = [4]float64{0.07, 0, 0.62, 0.52} // x0, y0, x1, y1
	tierCorner  = [4]float64{0.2, 0.5, 1, 1}
)

func cornerRect(cell image.Rectangle, f [4]float64) image.Rectangle {
	w, h := float64(cell.Dx()), float64(cell.Dy())
	return image.Rect(cell.Min.X+int(f[0]*w), cell.Min.Y+int(f[1]*h), cell.Min.X+int(f[2]*w+0.5), cell.Min.Y+int(f[3]*h+0.5))
}

// isWhite tells a figure's pixel on a large (4K) cell: bright and nearly
// grey. Only the font generator needs it; reading matches the figures'
// pictures instead, which also works where they are a few pixels thick.
func isWhite(r, g, b float32) bool {
	mn, mx := min(r, g, b), max(r, g, b)
	return mn > 165 && mx-mn < 70
}

// glyphBox is one connected run of white pixels.
type glyphBox struct {
	Rect image.Rectangle
	Area int
}

// glyphs finds the white figures in r of pic: 8-connected white pixels,
// kept when tall enough to be a figure (minH pixels), left to right.
func glyphs(pic Img, r image.Rectangle, minH int) []glyphBox {
	r = r.Intersect(image.Rect(0, 0, pic.W, pic.H))
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	white := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := ((r.Min.Y+y)*pic.W + r.Min.X + x) * 3
			white[y*w+x] = isWhite(pic.Pix[i], pic.Pix[i+1], pic.Pix[i+2])
		}
	}
	seen := make([]bool, w*h)
	var out []glyphBox
	stack := []int{}
	for start := range white {
		if !white[start] || seen[start] {
			continue
		}
		b := glyphBox{Rect: image.Rectangle{Min: image.Pt(w, h)}}
		seen[start] = true
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%w, p/w
			b.Area++
			b.Rect.Min.X, b.Rect.Min.Y = min(b.Rect.Min.X, x), min(b.Rect.Min.Y, y)
			b.Rect.Max.X, b.Rect.Max.Y = max(b.Rect.Max.X, x+1), max(b.Rect.Max.Y, y+1)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					q := ny*w + nx
					if white[q] && !seen[q] {
						seen[q] = true
						stack = append(stack, q)
					}
				}
			}
		}
		if b.Rect.Dy() >= minH {
			b.Rect = b.Rect.Add(r.Min)
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rect.Min.X < out[j].Rect.Min.X })
	return out
}

// figCell is the cell side figures are matched at (a 4K inventory's):
// corners are scaled so that their cell is this wide, whatever the screen.
// figOutline is how far (pixels at figCell) a figure's picture reaches past
// its white: its dark outline, so that a "1" does not fit inside another
// figure or along a bright edge of the icon.
const (
	figCell    = 105
	figOutline = 3
)

// Shares of the cell's side a number keeps to.
const (
	figEdge   = 0.35 // its first figure starts this near the cell's edge
	figMaxGap = 0.10 // between two figures of one number
	// figMinScore is how alike (normalized cross-correlation) a figure and
	// its picture in font must be; thinScore for "1" and "I", which any
	// bright upright edge of an icon resembles. An "I" beside a sure one,
	// level with it, may be nextLoss less alike: the icon's own white often
	// touches the outline of a tier's second or third bar.
	figMinScore = 0.65
	thinScore   = 0.88
	nextLoss    = 0.15
	nextOne     = 0.72
	// At the column where the grid's sure numbers start (see Anchors), a
	// figure needs only anchoredScore, a thin one anchoredThin.
	anchoredScore = 0.6
	anchoredThin  = 0.7
	anchorReach   = 3 // pixels at figCell, across
	anchorReachY  = 4 // and up or down: 6 and 8 rise above the others
	hitFloor      = 0.55
)

// figure is one of the game's figures as font keeps it: how white each
// pixel is (its darkest channel), outline included, at figCell.
type figure struct {
	R    rune
	W, H int
	Pix  []uint8
}

func (f figure) img() Img {
	m := newImg(f.W, f.H)
	for i, v := range f.Pix {
		m.Pix[i*3], m.Pix[i*3+1], m.Pix[i*3+2] = float32(v), float32(v), float32(v)
	}
	return m
}

var fontPrepared = func() []prepared {
	out := make([]prepared, len(font))
	for i, f := range font {
		if thin(f.R) {
			// A bar with its outline alone fits any bright edge; its whole
			// box, ground included, keeps it to figures.
			out[i] = prepareGrey(f.img())
		} else {
			out[i] = prepareGrey(f.masked())
		}
	}
	return out
}()

// masked is the figure with only its white and its outline counted (the
// white grown by outlineReach pixels): the ground beyond the outline is
// the icon, dark on one cell and white marble on another.
func (f figure) masked() Img {
	m := f.img()
	m.Mask = make([]float32, f.W*f.H)
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			if f.Pix[y*f.W+x] < 150 {
				continue
			}
			for dy := -outlineReach; dy <= outlineReach; dy++ {
				for dx := -outlineReach; dx <= outlineReach; dx++ {
					if nx, ny := x+dx, y+dy; nx >= 0 && ny >= 0 && nx < f.W && ny < f.H {
						m.Mask[ny*f.W+nx] = 1
					}
				}
			}
		}
	}
	return m
}

// outlineReach is how thick (pixels at figCell) the figures' dark outline
// is counted.
const outlineReach = 2

// whiteness keeps each pixel's darkest channel (in all three): white
// figures stay bright, coloured icon art goes dark.
func whiteness(m Img) Img {
	out := newImg(m.W, m.H)
	for i := 0; i < m.W*m.H; i++ {
		v := min(m.Pix[i*3], m.Pix[i*3+1], m.Pix[i*3+2])
		out.Pix[i*3], out.Pix[i*3+1], out.Pix[i*3+2] = v, v, v
	}
	return out
}

type hit struct {
	r     rune
	rect  image.Rectangle
	score float64
}

// findFigures matches every figure of font at every place of m (a corner,
// as whiteness at figCell) and keeps the best non-overlapping ones, left to
// right. allow limits which figures may be there.
func findFigures(m Img, allow func(rune) bool) []hit {
	var all []hit
	for i, f := range font {
		if !allow(f.R) {
			continue
		}
		t := fontPrepared[i]
		if !t.ok || t.w > m.W || t.h > m.H {
			continue
		}
		w, h := m.W-t.w+1, m.H-t.h+1
		scores := make([]float64, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				scores[y*w+x] = t.at(m, x, y)
			}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s := scores[y*w+x]
				if s < hitFloor || !localMax(scores, w, h, x, y) {
					continue
				}
				all = append(all, hit{f.R, image.Rect(x, y, x+t.w, y+t.h), s})
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	var kept []hit
	for _, a := range all {
		clash := false
		for _, k := range kept {
			if overlaps(a.rect, k.rect) {
				clash = true
				break
			}
		}
		if !clash {
			kept = append(kept, a)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].rect.Min.X < kept[j].rect.Min.X })
	return kept
}

func thin(r rune) bool { return r == '1' || r == 'I' }

func minFor(r rune) float64 {
	if thin(r) {
		return thinScore
	}
	return figMinScore
}

// edgeOf is how far a figure starts from its corner's outer edge: from the
// left for the stack size, from the right for the bottom-right number.
func edgeOf(h hit, width int, fromRight bool) float64 {
	if fromRight {
		return float64(width - h.rect.Max.X)
	}
	return float64(h.rect.Min.X)
}

func localMax(s []float64, w, h, x, y int) bool {
	v := s[y*w+x]
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx, ny := x+dx, y+dy
			if (dx != 0 || dy != 0) && nx >= 0 && ny >= 0 && nx < w && ny < h && s[ny*w+nx] > v {
				return false
			}
		}
	}
	return true
}

// overlaps tells two figures that claim the same ink: they share over a
// third of the narrower one's width and half the shorter one's height.
func overlaps(a, b image.Rectangle) bool {
	i := a.Intersect(b)
	if i.Empty() {
		return false
	}
	return float64(i.Dx()) > 0.34*float64(min(a.Dx(), b.Dx())) && float64(i.Dy()) > 0.5*float64(min(a.Dy(), b.Dy()))
}

// runHits keeps the figures of one number: the run that starts at the
// corner's outer edge (left for the stack size, right for the bottom
// one), figure beside figure and level with each other, left to right.
//
// With an anchor (where this grid's numbers start, see Anchors) the first
// figure must start there and may be less alike (anchoredScore); without
// one it is the one nearest the corner that is a sure match. Each next
// figure is the nearest one just beyond the last, the best match where a
// weak and a sure one sit at the same place.
func runHits(hits []hit, side float64, width int, fromRight bool, a anchor) []hit {
	if len(hits) == 0 {
		return nil
	}
	start := -1
	if a.ok() {
		for i, h := range hits {
			need := anchoredScore
			if thin(h.r) {
				need = anchoredThin
			}
			if math.Abs(edgeOf(h, width, fromRight)-a.X) <= anchorReach && math.Abs(lineOf(h, fromRight)-a.Y) <= anchorReachY &&
				h.score >= need && (start < 0 || h.score > hits[start].score) {
				start = i
			}
		}
	} else {
		best := math.Inf(1)
		for i, h := range hits {
			if e := edgeOf(h, width, fromRight); h.score >= minFor(h.r) && e < best {
				start, best = i, e
			}
		}
		if start >= 0 && best > figEdge*side {
			start = -1
		}
	}
	if start < 0 {
		return nil
	}
	first := hits[start]
	run := []hit{first}
	used := map[int]bool{start: true}
	for {
		prev := run[len(run)-1]
		gaps := map[int]float64{}
		closest := math.Inf(1)
		for i, h := range hits {
			if used[i] {
				continue
			}
			gap := float64(h.rect.Min.X - prev.rect.Max.X)
			if fromRight {
				gap = float64(prev.rect.Min.X - h.rect.Max.X)
			}
			v := h.rect.Intersect(image.Rect(h.rect.Min.X, first.rect.Min.Y, h.rect.Max.X, first.rect.Max.Y))
			if gap < -2*figOutline || gap > figMaxGap*side || float64(v.Dy()) < 0.6*float64(min(h.rect.Dy(), first.rect.Dy())) {
				continue
			}
			gaps[i] = gap
			closest = math.Min(closest, gap)
		}
		// The nearest figure (a III's middle bar, not the far one), the best
		// match among those about as near.
		next := -1
		for i, gap := range gaps {
			if gap <= closest+figOutline && (next < 0 || hits[i].score > hits[next].score) {
				next = i
			}
		}
		if next < 0 {
			break
		}
		h := hits[next]
		// A "1" pressed against the figure before it, on its line ("11"),
		// may be as little alike as nextOne.
		tight := h.r == '1' && gaps[next] <= 2 && math.Abs(float64(h.rect.Min.Y-prev.rect.Min.Y)) <= 3 && h.score >= nextOne
		if need := minFor(h.r); h.score < need && !tight && (h.r != 'I' || h.score < need-nextLoss) {
			break
		}
		used[next] = true
		run = append(run, h)
	}
	if fromRight {
		for i, j := 0, len(run)-1; i < j; i, j = i+1, j-1 {
			run[i], run[j] = run[j], run[i]
		}
	}
	return run
}

// anchor is where a grid's numbers of one corner start: X as edgeOf
// measures it, Y the line they keep (lineOf). Negative: not known.
type anchor struct{ X, Y float64 }

func (a anchor) ok() bool { return a.X >= 0 && a.Y >= 0 }

var noAnchor = anchor{-1, -1}

// lineOf is the line a corner's figures keep: their top for the stack size
// (old-style figures reach below it unevenly), their bottom for the
// bottom-right number.
func lineOf(h hit, fromRight bool) float64 {
	if fromRight {
		return float64(h.rect.Max.Y)
	}
	return float64(h.rect.Min.Y)
}

// cornerScan is one corner of a cell as figures are matched in it.
type cornerScan struct {
	sub  Img
	hits []hit
}

func scanCorner(pic Img, cell image.Rectangle, corner [4]float64, allow func(rune) bool) cornerScan {
	r := cornerRect(cell, corner).Intersect(image.Rect(0, 0, pic.W, pic.H))
	if r.Empty() {
		return cornerScan{}
	}
	k := float64(figCell) / float64(cell.Dx())
	sub := whiteness(pic.crop(r))
	sub = sub.Resize(max(1, int(float64(sub.W)*k+0.5)), max(1, int(float64(sub.H)*k+0.5)))
	return cornerScan{sub, findFigures(sub, allow)}
}

// read reads the corner's number. unsure is set when a white shape the
// size of a figure, next to the number or alone at the corner, matched no
// figure: a figure font does not know yet, whose number would otherwise
// be read short ("XIX" as "I").
func (c cornerScan) read(fromRight bool, a anchor) (text string, unsure bool, run []hit) {
	if c.sub.W == 0 {
		return "", false, nil
	}
	run = runHits(c.hits, float64(figCell), c.sub.W, fromRight, a)
	for _, h := range run {
		text += string(h.r)
	}
	return text, strayFigure(c.sub, run, fromRight, a), run
}

// outlined tells a shape drawn the way the game writes figures: the ring
// just outside its box is mostly dark (the figures' outline). The icon's
// own bright parts sit among mid tones.
func outlined(m Img, r image.Rectangle) bool {
	dark, n := 0, 0
	ring := r.Inset(-2).Intersect(image.Rect(0, 0, m.W, m.H))
	for y := ring.Min.Y; y < ring.Max.Y; y++ {
		for x := ring.Min.X; x < ring.Max.X; x++ {
			if image.Pt(x, y).In(r) {
				continue
			}
			n++
			if m.Pix[(y*m.W+x)*3] < 70 {
				dark++
			}
		}
	}
	return n > 0 && float64(dark) >= 0.6*float64(n)
}

// strayFigure tells a figure-sized white shape beside the run (or, when
// the run is empty, where the grid's numbers stand) that the run does not
// cover.
func strayFigure(sub Img, run []hit, fromRight bool, a anchor) bool {
	side := float64(figCell)
	for _, b := range glyphs(sub, image.Rect(0, 0, sub.W, sub.H), int(0.15*side)) {
		h, w := float64(b.Rect.Dy()), float64(b.Rect.Dx())
		if h > 0.27*side || w > 0.18*side || !outlined(sub, b.Rect) {
			continue
		}
		covered := false
		for _, x := range run {
			if overlaps(b.Rect, x.rect.Inset(figOutline)) || b.Rect.In(x.rect) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		if len(run) == 0 {
			edge, line := float64(b.Rect.Min.X), float64(b.Rect.Min.Y)
			if fromRight {
				edge, line = float64(sub.W-b.Rect.Max.X), float64(b.Rect.Max.Y)
			}
			// Where this grid's numbers stand, when that is known.
			if a.ok() {
				if math.Abs(edge-a.X) <= 2*anchorReach && math.Abs(line-a.Y) <= anchorReachY {
					return true
				}
			} else if edge <= figEdge*side {
				return true
			}
			continue
		}
		// Beside the run's inner end, level with it.
		last := run[len(run)-1].rect
		gap := float64(b.Rect.Min.X - last.Max.X)
		if fromRight {
			last = run[0].rect
			gap = float64(last.Min.X - b.Rect.Max.X)
		}
		// On the number's line: its top (stack size) or bottom (corner).
		line := math.Abs(float64(b.Rect.Min.Y - last.Min.Y))
		if fromRight {
			line = math.Abs(float64(b.Rect.Max.Y - last.Max.Y))
		}
		if gap >= -2 && gap <= figMaxGap*side && line <= anchorReachY {
			return true
		}
	}
	return false
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// Marks is what the game writes on a cell.
type Marks struct {
	// Stack is the stack size (1 when none is written: a single item).
	Stack int
	// Corner is the number written bottom right (0 for none): an orb's
	// tier (II, III), a gem's or flux's level (XIX), a tablet's uses (9).
	// Roman tells it was written in Roman numerals. Which item of an icon
	// it names is Pick's to say.
	Corner int
	Roman  bool
	// Unsure is set when a figure could not be read; the numbers may be
	// short of one.
	Unsure bool
}

func isRoman(r rune) bool { return r == 'I' || r == 'V' || r == 'X' }

// ReadMarks reads the stack size and the bottom-right figures of cell (the
// cell's own rectangle in pic, as Grid.Rect gives it; the corners reach a
// little past it, since the grid is found to within a few pixels).
func ReadMarks(pic Img, cell image.Rectangle) Marks {
	st, co := scanMarks(pic, cell)
	return marksOf(st, co, NoAnchors)
}

// Anchors are where a grid's numbers start: the stack sizes and the
// bottom-right numbers.
type Anchors struct{ Stack, Corner anchor }

var NoAnchors = Anchors{noAnchor, noAnchor}

func scanMarks(pic Img, cell image.Rectangle) (stack, corner cornerScan) {
	wide := cell.Inset(-Pad(float64(cell.Dx())) / 2)
	return scanCorner(pic, wide, stackCorner, isDigit), scanCorner(pic, wide, tierCorner, func(r rune) bool { return isDigit(r) || isRoman(r) })
}

func marksOf(stack, corner cornerScan, a Anchors) Marks {
	m := Marks{Stack: 1}
	s, unsure, _ := stack.read(false, a.Stack)
	if n, ok := number(s); ok && n > 0 {
		m.Stack = n
	}
	t, unsure2, _ := corner.read(true, a.Corner)
	m.Unsure = unsure || unsure2
	if n, ok := roman(t); ok {
		m.Corner, m.Roman = n, true
	} else if n, ok := number(t); ok {
		m.Corner = n
	} else if t != "" {
		m.Unsure = true
	}
	return m
}

// anchorsOf measures where the sure numbers of a grid start: the median
// over cells whose first figure is a wide one matched well (a "1" or "I"
// is too easily matched elsewhere to place the column).
func anchorsOf(scans [][2]cornerScan) Anchors {
	var sx, sy, cx, cy []float64
	for _, sc := range scans {
		if _, _, run := sc[0].read(false, noAnchor); len(run) > 0 && !thin(run[0].r) && run[0].score >= 0.8 {
			sx = append(sx, edgeOf(run[0], sc[0].sub.W, false))
			sy = append(sy, lineOf(run[0], false))
		}
		if _, _, run := sc[1].read(true, noAnchor); len(run) > 0 && run[len(run)-1].score >= 0.85 {
			h := run[len(run)-1]
			cx = append(cx, edgeOf(h, sc[1].sub.W, true))
			cy = append(cy, lineOf(h, true))
		}
	}
	a := NoAnchors
	if x, y := median(sx), median(sy); x >= 0 && y >= 0 {
		a.Stack = anchor{x, y}
	}
	if x, y := median(cx), median(cy); x >= 0 && y >= 0 {
		a.Corner = anchor{x, y}
	}
	return a
}

// median of at least two values, else −1.
func median(v []float64) float64 {
	if len(v) < 2 {
		return -1
	}
	v = append([]float64(nil), v...)
	sort.Float64s(v)
	return v[len(v)/2]
}

// roman reads I, V and X numerals (the game's levels go to XX).
func roman(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	val := map[rune]int{'I': 1, 'V': 5, 'X': 10}
	rs := []rune(s)
	n := 0
	for i, r := range rs {
		v, ok := val[r]
		if !ok {
			return 0, false
		}
		if i+1 < len(rs) && val[rs[i+1]] > v {
			n -= v
		} else {
			n += v
		}
	}
	return n, n > 0
}

func number(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if !isDigit(r) {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// figureBoxes finds the figures on a large (4K) cell by their white
// pixels, in reading order: what the font generator cuts them from.
func figureBoxes(pic Img, cell image.Rectangle, corner [4]float64) []image.Rectangle {
	side := float64(cell.Dx())
	var out []image.Rectangle
	for _, b := range glyphs(pic, cornerRect(cell, corner), int(0.15*side)) {
		if h, w := float64(b.Rect.Dy()), float64(b.Rect.Dx()); h <= 0.27*side && w <= 0.18*side {
			out = append(out, b.Rect)
		}
	}
	return out
}
