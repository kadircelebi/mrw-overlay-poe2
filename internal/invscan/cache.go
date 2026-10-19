package invscan

import "math"

// GridCache keeps what each cell of a grid was read as, with a small
// picture of it: read again and again (the inventory is watched while it is
// open), a grid mostly holds what it held, and only the cells that changed
// need the full match.
type GridCache struct {
	cells map[[2]int]cachedCell
}

type cachedCell struct {
	print  []float32
	empty  bool
	result Result
	scans  [2]cornerScan
}

// printSide is the side of a cell's small picture (grey, averaged).
const printSide = 12

// printTolerance is how much (mean brightness, 0–255) a cell's small
// picture may differ and still be the same cell: the screen's own noise,
// not a stack's number changing (which moves far more).
const printTolerance = 3

func cellPrint(cell Img) []float32 {
	small := cell.Resize(printSide, printSide)
	out := make([]float32, printSide*printSide)
	for i := range out {
		out[i] = (small.Pix[i*3] + small.Pix[i*3+1] + small.Pix[i*3+2]) / 3
	}
	return out
}

func samePrint(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	var d float64
	for i := range a {
		d = math.Max(d, math.Abs(float64(a[i]-b[i])))
	}
	return d <= 4*printTolerance
}

// ReadGridCached is ReadGrid reusing c's reads of the cells that look as
// they did; it keeps the new ones in c.
func (m *Matcher) ReadGridCached(pic Img, g Grid, c *GridCache) []Cell {
	if c.cells == nil {
		c.cells = map[[2]int]cachedCell{}
	}
	pad := Pad(g.Cell)
	type job struct {
		at    [2]int
		print []float32
	}
	var cells []Cell
	var scans [][2]cornerScan
	var todo []job
	var todoIdx []int
	seen := map[[2]int]bool{}
	for r := 0; r < g.Rows; r++ {
		for col := 0; col < g.Cols; col++ {
			at := [2]int{r, col}
			seen[at] = true
			cell := pic.crop(g.Rect(col, r))
			p := cellPrint(cell)
			if old, ok := c.cells[at]; ok && samePrint(old.print, p) {
				if !old.empty {
					cells = append(cells, Cell{Col: col, Row: r, Result: old.result})
					scans = append(scans, old.scans)
				}
				continue
			}
			if Empty(cell) {
				c.cells[at] = cachedCell{print: p, empty: true}
				continue
			}
			cells = append(cells, Cell{Col: col, Row: r})
			scans = append(scans, [2]cornerScan{})
			todo = append(todo, job{at, p})
			todoIdx = append(todoIdx, len(cells)-1)
		}
	}
	for at := range c.cells {
		if !seen[at] {
			delete(c.cells, at)
		}
	}
	parallel(len(todo), func(k int) {
		i := todoIdx[k]
		cl := &cells[i]
		cl.Result = m.Classify(pic.crop(g.Padded(cl.Col, cl.Row, pad)))
		if cl.Icon >= 0 {
			scans[i][0], scans[i][1] = scanMarks(pic, g.Rect(cl.Col, cl.Row))
		}
	})
	for k, j := range todo {
		i := todoIdx[k]
		c.cells[j.at] = cachedCell{print: j.print, result: cells[i].Result, scans: scans[i]}
	}
	var listed [][2]cornerScan
	for i, cl := range cells {
		if cl.Icon >= 0 {
			listed = append(listed, scans[i])
		}
	}
	a := anchorsOf(listed)
	for i := range cells {
		if cells[i].Icon >= 0 {
			cells[i].Marks = marksOf(scans[i][0], scans[i][1], a)
		}
	}
	return cells
}
