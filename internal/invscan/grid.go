package invscan

import (
	"image"
	"math"
)

// The inventory's 12×5 grid, measured on a 3840×2160 game (2026-10-18):
// cells of 105.3 px from (2544, 1181). The game sizes its panels by the
// window's height and keeps the inventory at the right edge, so the grid is
// given as shares of the height, its left edge counted from the right.
const (
	InventoryCols = 12
	InventoryRows = 5

	invCell  = 105.3 / 2160
	invLeft  = (3840 - 2544) / 2160.0 // from the right edge
	invTop   = 1181 / 2160.0
	gridSlop = 0.25 // of a cell, searched around the expected place
)

// Grid is where cells lie in a picture: the top-left corner of the first
// cell and the cell's side, both in pixels.
type Grid struct {
	X, Y, Cell float64
	Cols, Rows int
}

// Rect is cell (col, row) in the picture.
func (g Grid) Rect(col, row int) image.Rectangle {
	x, y := g.X+float64(col)*g.Cell, g.Y+float64(row)*g.Cell
	return image.Rect(int(math.Round(x)), int(math.Round(y)), int(math.Round(x+g.Cell)), int(math.Round(y+g.Cell)))
}

// Padded is cell (col, row) with pad pixels more on each side.
func (g Grid) Padded(col, row, pad int) image.Rectangle {
	return g.Rect(col, row).Inset(-pad)
}

// InventoryGuess is where the inventory grid should be in a game client
// area of w×h pixels.
func InventoryGuess(w, h int) Grid {
	H := float64(h)
	return Grid{X: float64(w) - invLeft*H, Y: invTop * H, Cell: invCell * H, Cols: InventoryCols, Rows: InventoryRows}
}

// Align finds the grid near guess in pic: the lines between cells are
// darker than the cells, so it looks for the cell size and offset (within a
// few percent and a quarter cell of the guess) whose lines are darkest on
// average, across and down separately.
func Align(pic Img, guess Grid) Grid {
	g := guess
	g.Cell, g.X = alignAxis(pic, guess, true)
	cellY, y := alignAxis(pic, guess, false)
	g.Y = y
	g.Cell = (g.Cell + cellY) / 2
	return g
}

func alignAxis(pic Img, g Grid, across bool) (cell, start float64) {
	lines, start0 := g.Cols+1, g.X
	if !across {
		lines, start0 = g.Rows+1, g.Y
	}
	// Brightness profile along the axis, over the grid's span on the other.
	var prof []float64
	if across {
		y0, y1 := clamp(int(g.Y), pic.H-1), clamp(int(g.Y+float64(g.Rows)*g.Cell), pic.H)
		prof = make([]float64, pic.W)
		for x := 0; x < pic.W; x++ {
			var s float64
			for y := y0; y < y1; y++ {
				i := (y*pic.W + x) * 3
				s += float64(pic.Pix[i] + pic.Pix[i+1] + pic.Pix[i+2])
			}
			prof[x] = s / float64(max(1, y1-y0))
		}
	} else {
		x0, x1 := clamp(int(g.X), pic.W-1), clamp(int(g.X+float64(g.Cols)*g.Cell), pic.W)
		prof = make([]float64, pic.H)
		for y := 0; y < pic.H; y++ {
			var s float64
			for x := x0; x < x1; x++ {
				i := (y*pic.W + x) * 3
				s += float64(pic.Pix[i] + pic.Pix[i+1] + pic.Pix[i+2])
			}
			prof[y] = s / float64(max(1, x1-x0))
		}
	}
	// Smooth over the gap's width so a line's darkest point is its middle.
	prof = boxBlur(prof, max(1, int(g.Cell*0.04)))
	best := math.Inf(1)
	cell, start = g.Cell, start0
	for c := g.Cell * 0.96; c <= g.Cell*1.04; c += 0.05 {
		for s := start0 - gridSlop*g.Cell; s <= start0+gridSlop*g.Cell; s += 0.5 {
			var sum float64
			ok := true
			for k := 0; k < lines; k++ {
				p := int(math.Round(s + float64(k)*c))
				if p < 0 || p >= len(prof) {
					ok = false
					break
				}
				sum += prof[p]
			}
			if ok && sum < best {
				best, cell, start = sum, c, s
			}
		}
	}
	return cell, start
}

func boxBlur(p []float64, r int) []float64 {
	out := make([]float64, len(p))
	for i := range p {
		var s float64
		n := 0
		for j := max(0, i-r); j <= min(len(p)-1, i+r); j++ {
			s += p[j]
			n++
		}
		out[i] = s / float64(n)
	}
	return out
}

// Empty tells an empty cell: the game draws empty ones dark and grey
// (a faint pattern), filled ones on a blue ground with a picture over it.
func Empty(cell Img) bool {
	m := int(float64(min(cell.W, cell.H)) * 0.1)
	var sr, sb, s, ss float64
	n := 0
	for y := m; y < cell.H-m; y++ {
		for x := m; x < cell.W-m; x++ {
			i := (y*cell.W + x) * 3
			r, g, b := float64(cell.Pix[i]), float64(cell.Pix[i+1]), float64(cell.Pix[i+2])
			sr += r
			sb += b
			for _, v := range [3]float64{r, g, b} {
				s += v
				ss += v * v
			}
			n++
		}
	}
	if n == 0 {
		return true
	}
	blue := (sb - sr) / float64(n)
	mean := s / float64(3*n)
	std := math.Sqrt(math.Max(ss/float64(3*n)-mean*mean, 0))
	return blue < 12 && std < 28
}

// Shown tells whether g lies on an item grid at all (the inventory may be
// closed, the game world behind): nearly every cell is either empty or
// holds an item on the game's blue ground, seen in the corners the numbers
// leave free (top right, bottom left). Empty cells are nearly black, as is
// the screen while the game fades between areas, so where two empty cells
// meet most must show the bright line the panel draws between them.
func Shown(pic Img, g Grid) bool {
	ok, n := 0, 0
	empty := make([]bool, g.Rows*g.Cols)
	for r := 0; r < g.Rows; r++ {
		for c := 0; c < g.Cols; c++ {
			cell := g.Rect(c, r)
			n++
			if Empty(pic.crop(cell)) {
				empty[r*g.Cols+c] = true
				ok++
				continue
			}
			s := max(2, cell.Dx()*12/100)
			tr := image.Rect(cell.Max.X-2*s, cell.Min.Y+s/2, cell.Max.X-s/2, cell.Min.Y+2*s)
			bl := image.Rect(cell.Min.X+s/2, cell.Max.Y-2*s, cell.Min.X+2*s, cell.Max.Y-s/2)
			if blueGround(pic, tr) || blueGround(pic, bl) {
				ok++
			}
		}
	}
	if n == 0 || ok*10 < n*8 {
		return false
	}
	pairs, lined := 0, 0
	for r := 0; r < g.Rows; r++ {
		for c := 0; c+1 < g.Cols; c++ {
			if empty[r*g.Cols+c] && empty[r*g.Cols+c+1] {
				pairs++
				if gridLine(pic, g, c+1, r) {
					lined++
				}
			}
		}
	}
	return pairs < 2 || lined*10 >= pairs*6
}

// gridLine tells whether the panel's line shows where column c begins in
// row r: some column near it well brighter than the cells either side.
func gridLine(pic Img, g Grid, c, r int) bool {
	x := g.X + float64(c)*g.Cell
	reach := max(3, int(g.Cell*0.12+0.5))
	y0 := clamp(int(g.Y+(float64(r)+0.2)*g.Cell), pic.H)
	y1 := clamp(int(g.Y+(float64(r)+0.8)*g.Cell), pic.H)
	if y1 <= y0 {
		return false
	}
	col := func(px int) float64 {
		px = clamp(px, pic.W-1)
		var s float64
		for y := y0; y < y1; y++ {
			i := (y*pic.W + px) * 3
			s += float64(pic.Pix[i] + pic.Pix[i+1] + pic.Pix[i+2])
		}
		return s / float64(3*(y1-y0))
	}
	var peak float64
	for d := -reach; d <= reach; d++ {
		peak = max(peak, col(int(x+0.5)+d))
	}
	side := (col(int(x-0.3*g.Cell)) + col(int(x+0.3*g.Cell))) / 2
	return peak-side > 15
}

// blueGround tells the dark blue an item's cell is drawn on.
func blueGround(pic Img, r image.Rectangle) bool {
	p := pic.crop(r)
	var sr, sb float64
	for i := 0; i < p.W*p.H; i++ {
		sr += float64(p.Pix[i*3])
		sb += float64(p.Pix[i*3+2])
	}
	n := float64(max(1, p.W*p.H))
	return sb/n-sr/n > 12 && sb/n > 28
}
