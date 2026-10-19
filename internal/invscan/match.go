package invscan

import (
	"image"
	"runtime"
	"sort"
	"sync"
)

// Entry is one item an icon stands for. Several share an icon: the three
// tiers of an orb (told apart by the II/III the game writes on them), the
// levels of an uncut gem.
type Entry struct {
	ID    string // trade id ("exalted", "greater-exalted-orb")
	Name  string // English name
	Group string // exchange section ("Currency", "Essences")
}

// Icon is an item picture as the trade site (or another source) serves it,
// transparent around the item.
type Icon struct {
	Key     string
	Entries []Entry
	Image   image.Image
}

// Result is what a filled cell shows.
type Result struct {
	// Icon indexes the matcher's icons; -1 when nothing matched well enough
	// (an item outside the list: gear, tablets, waystones).
	Icon    int
	Entries []Entry
	// Score is the shape match (normalized cross-correlation, 1 = same
	// picture), Color the colour signature's (1 = same), Margin how far the
	// pick is ahead of the next icon on their sum.
	Score, Color, Margin float64
	// Second is the runner-up icon (-1 when there is none).
	Second int
}

// Thresholds measured on a 4K inventory (2026-10-18): listed items scored
// 0.82–0.98, others (tablets, rings, waystones) 0.39 at most.
const (
	minScore = 0.6
	// colorWeight adds the colour signature to the shape score: essences
	// and liquids of one shape differ mainly in colour.
	colorWeight = 0.5
)

// Scales of the icon's frame to the cell: the trade site's icons fill
// about 95% of an inventory cell, poe2db's larger frames about 99%.
var fineScales = []float64{0.93, 0.95, 0.97, 0.99}

const (
	coarseCell = 40 // the cell's side in the first, rough pass
	coarseKeep = 12
	// fineReach is how far (pixels) the full-size pass looks around the
	// place the rough pass found.
	fineReach   = 4
	coarseScale = 0.95
)

// Matcher tells which icon a cell shows. It keeps the icons at the sizes it
// needs; one matcher serves one cell size.
type Matcher struct {
	icons  []Icon
	src    []Img // icons as loaded
	cell   int
	coarse []Img

	mu   sync.Mutex
	fine map[[2]int]Img // icon index, size
}

// NewMatcher prepares the icons for cells of the given side in pixels.
func NewMatcher(icons []Icon, cell int) *Matcher {
	m := &Matcher{icons: icons, cell: cell, fine: map[[2]int]Img{}}
	n := int(coarseCell * coarseScale)
	for _, ic := range icons {
		src := fromIcon(ic.Image)
		m.src = append(m.src, src)
		m.coarse = append(m.coarse, fit(src, n))
	}
	return m
}

// fit scales an icon's frame into an n×n template, keeping its aspect
// (centred), and masks the corners where the game writes numbers: the stack
// size top left, a gem's level or a tier's II/III bottom right.
func fit(src Img, n int) Img {
	w, h := n, n
	if src.W > src.H {
		h = max(1, n*src.H/src.W)
	} else if src.H > src.W {
		w = max(1, n*src.W/src.H)
	}
	r := src.Resize(w, h)
	out := newImg(n, n)
	out.Mask = make([]float32, n*n)
	ox, oy := (n-w)/2, (n-h)/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i, o := y*w+x, (oy+y)*n+ox+x
			copy(out.Pix[o*3:o*3+3], r.Pix[i*3:i*3+3])
			out.Mask[o] = r.Mask[i]
		}
	}
	maskCorners(out.Mask, n)
	return out
}

func maskCorners(mask []float32, n int) {
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fy, fx := float64(y)/float64(n), float64(x)/float64(n)
			if (fy < 0.30 && fx < 0.45) || (fy >= 0.72 && fx >= 0.62) {
				mask[y*n+x] = 0
			}
		}
	}
}

func (m *Matcher) fineTemplate(i, n int) Img {
	m.mu.Lock()
	t, ok := m.fine[[2]int{i, n}]
	m.mu.Unlock()
	if ok {
		return t
	}
	t = fit(m.src[i], n)
	m.mu.Lock()
	m.fine[[2]int{i, n}] = t
	m.mu.Unlock()
	return t
}

// Pad is how much wider than a cell to cut each side of it: the grid is
// found to within a few pixels, and the icon is searched around.
func Pad(cell float64) int { return int(cell*0.08 + 0.5) }

// Classify tells which icon a cell shows; cell is the cell cut with Pad
// around it. Two passes: a rough shape match against every icon at a small
// size, which also finds where the icon sits, then the full-size match of
// the closest ones near that place at a few scales, with the colour of the
// matched pixels added.
func (m *Matcher) Classify(cell Img) Result {
	if len(m.icons) == 0 {
		return Result{Icon: -1}
	}
	f := float64(coarseCell) / float64(m.cell)
	small := cell.Resize(max(1, int(float64(cell.W)*f+0.5)), max(1, int(float64(cell.H)*f+0.5)))
	type cand struct {
		i     int
		score float64
		at    image.Point
	}
	rough := make([]cand, len(m.icons))
	for i := range m.icons {
		s, at := NCC(small, m.coarse[i])
		rough[i] = cand{i, s, at}
	}
	sort.Slice(rough, func(a, b int) bool { return rough[a].score > rough[b].score })
	rough = rough[:min(coarseKeep, len(rough))]

	type scored struct {
		i                   int
		shape, color, total float64
	}
	half := float64(m.coarse[0].W) / 2
	var all []scored
	for _, c := range rough {
		best := scored{i: c.i, shape: -1}
		// The icon's centre as the rough pass found it, in cell pixels.
		cx, cy := (float64(c.at.X)+half)/f, (float64(c.at.Y)+half)/f
		for _, sc := range fineScales {
			n := int(float64(m.cell)*sc + 0.5)
			if n > cell.W || n > cell.H {
				continue
			}
			t := m.fineTemplate(c.i, n)
			x0, y0 := int(cx-float64(n)/2+0.5), int(cy-float64(n)/2+0.5)
			s, at := nccIn(cell, t, image.Rect(x0-fineReach, y0-fineReach, x0+fineReach+1, y0+fineReach+1))
			if s > best.shape {
				win := cell.crop(image.Rect(at.X, at.Y, at.X+n, at.Y+n))
				best.shape = s
				best.color = dot(hueHist(win, t.Mask), hueHist(t, t.Mask))
			}
		}
		best.total = best.shape + colorWeight*best.color
		all = append(all, best)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].total > all[b].total })
	top := all[0]
	res := Result{Icon: top.i, Entries: m.icons[top.i].Entries, Score: top.shape, Color: top.color, Second: -1}
	if len(all) > 1 {
		res.Margin = top.total - all[1].total
		res.Second = all[1].i
	}
	if top.shape < minScore {
		res.Icon, res.Entries = -1, nil
	}
	return res
}

// ClassifyAll runs Classify on the cells in parallel, in order.
func (m *Matcher) ClassifyAll(cells []Img) []Result {
	out := make([]Result, len(cells))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < max(1, runtime.NumCPU()-1); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i] = m.Classify(cells[i])
			}
		}()
	}
	for i := range cells {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

// Cell is what one cell of a grid holds: Icon −1 for an item off the list,
// and only listed items have their marks read.
type Cell struct {
	Col, Row int
	Result
	Marks
}

// ReadGrid reads every filled cell of g in pic, in parallel, row by row.
// Numbers are read in two passes: the first finds where the grid's sure
// numbers start (anchorsOf), the second reads every number from there.
func (m *Matcher) ReadGrid(pic Img, g Grid) []Cell {
	var cells []Cell
	for r := 0; r < g.Rows; r++ {
		for c := 0; c < g.Cols; c++ {
			if !Empty(pic.crop(g.Rect(c, r))) {
				cells = append(cells, Cell{Col: c, Row: r})
			}
		}
	}
	pad := Pad(g.Cell)
	scans := make([][2]cornerScan, len(cells))
	parallel(len(cells), func(i int) {
		c := &cells[i]
		c.Result = Result{Icon: -1}
		if !Bare(pic, g.Rect(c.Col, c.Row)) {
			c.Result = m.Classify(pic.crop(g.Padded(c.Col, c.Row, pad)))
		}
		if c.Icon >= 0 {
			scans[i][0], scans[i][1] = scanMarks(pic, g.Rect(c.Col, c.Row))
		}
	})
	var listed [][2]cornerScan
	for i, c := range cells {
		if c.Icon >= 0 {
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

// parallel runs f(0..n−1) on all but one of the processors.
func parallel(n int, f func(int)) {
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < max(1, runtime.NumCPU()-1); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				f(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
}
