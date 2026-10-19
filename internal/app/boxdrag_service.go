package app

import (
	"image"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/overlay"
)

// The boxes over the game ("loot", "expedition", "earnings") let clicks
// through to it, except for each one's grip: a small window just outside
// its top-right corner. Dragging the grip moves the box, and letting go
// keeps the box there. While a box is being placed from the tracker window
// the box itself takes the mouse and is dragged the same way.
//
// Dragging is done by the pages, not Windows' move loop: the boxes never
// take the focus, so the game stays in front, and a move asked for after
// the button is already up would leave Windows moving the box with every
// click going to it. The page sends each drag's moves numbered from 1; the
// first to arrive takes the box's place as the drag's start, and a move
// older than one already made is dropped.

var boxNames = []string{"loot", "expedition", "earnings"}

// The grip's size and its gap to the box, in window pixels.
const (
	gripWidth  = 14
	gripHeight = 11
	gripGap    = 2
)

type boxDragState struct {
	// moving keeps one move's windows from being laid between another's
	// (mu, taken inside it, guards the state below and the grips').
	moving sync.Mutex
	mu     sync.Mutex
	box    string // the box a grip is moving ("" when none)
	drag   int
	seq    int
	start  image.Rectangle
	// ended is the last drag let go of: its moves still on their way (they
	// arrive one by one, in any order) move nothing. movedAt is when box
	// last moved: a drag whose end never came stops counting after
	// boxDragIdle.
	ended   int
	movedAt time.Time
	// grips that Wails has shown once (later shown and put away without
	// it), and those showing now.
	gripReady map[string]bool
	gripOn    map[string]bool
}

// A grip is shown by Wails only once: its showing activates the window,
// and the grip, unlike the boxes, takes the mouse, so the game would lose
// the keys (and the scan, that reads only while the game is in front).
// After that it is laid over the game without being activated, and put
// away off the screen rather than hidden: a hidden WebView can leave its
// place taking clicks.
var gripAway = image.Rect(-32000, -32000, -32000+gripWidth, -32000+gripHeight)

// boxWindow is the window of box, and whether it is being placed.
func (s *AppService) boxWindow(box string) (application.Window, bool) {
	switch box {
	case "loot":
		return s.lootBoxWindow, s.lootPlacing.Load()
	case "expedition":
		return s.expeditionStripWindow, s.expeditionPlacing.Load()
	case "earnings":
		return s.earningsWindow, s.earningsPlacing.Load()
	}
	return nil, false
}

// boxDragIdle is how long a grip's drag counts without a move.
const boxDragIdle = 2 * time.Second

// boxMoving tells whether box's grip is being dragged: its place then is
// the drag's, not the settings'.
func (s *AppService) boxMoving(box string) bool {
	s.boxDrag.mu.Lock()
	defer s.boxDrag.mu.Unlock()
	return s.boxDrag.box == box && time.Since(s.boxDrag.movedAt) < boxDragIdle
}

// MoveBox moves box by dx, dy physical pixels from where it was when drag
// began (move seq of that drag), kept inside the game's client area: while
// it is being placed, or by its grip while it shows.
func (s *AppService) MoveBox(box string, drag, seq, dx, dy int) {
	s.moveBox(box, drag, seq, dx, dy)
}

// EndBoxMove makes the last move of a grip's drag and keeps the box's new
// place, as a share of the game's client area.
func (s *AppService) EndBoxMove(box string, drag, seq, dx, dy int) {
	w, placing := s.moveBox(box, drag, seq, dx, dy)
	s.boxDrag.mu.Lock()
	s.boxDrag.ended = drag
	if s.boxDrag.box == box {
		s.boxDrag.box = ""
	}
	s.boxDrag.mu.Unlock()
	if w == nil || placing {
		return
	}
	r, okWin := overlay.WindowRect(uintptr(w.NativeWindow()))
	client, okGame := overlay.GameClient()
	if !okWin || !okGame || client.Dx() <= 0 || client.Dy() <= 0 {
		return
	}
	fx := float64(r.Min.X-client.Min.X) / float64(client.Dx())
	fy := float64(r.Min.Y-client.Min.Y) / float64(client.Dy())
	next := s.GetOverlaySettings()
	switch box {
	case "loot":
		next.LootCorner, next.LootX, next.LootY = overlay.EarningsCustom, fx, fy
	case "expedition":
		next.ExpeditionCorner, next.ExpeditionX, next.ExpeditionY = overlay.EarningsCustom, fx, fy
	case "earnings":
		next.EarningsCorner, next.EarningsX, next.EarningsY = overlay.EarningsCustom, fx, fy
	}
	if _, err := s.SaveOverlaySettings(next); err != nil {
		log.Printf("%s box: save place: %v", box, err)
	}
	s.syncBoxes()
}

func (s *AppService) moveBox(box string, drag, seq, dx, dy int) (application.Window, bool) {
	w, placing := s.boxWindow(box)
	if w == nil || !placing && !w.IsVisible() {
		return nil, placing
	}
	hwnd := uintptr(w.NativeWindow())
	st := &s.boxDrag
	st.moving.Lock()
	defer st.moving.Unlock()
	start, ok := st.moveFrom(box, drag, seq, placing, hwnd)
	if !ok {
		return w, placing
	}
	r := start.Add(image.Pt(dx, dy))
	client, ok := overlay.GameClient()
	if ok {
		r = r.Add(image.Pt(max(0, client.Min.X-r.Min.X), max(0, client.Min.Y-r.Min.Y)))
		r = r.Sub(image.Pt(max(0, r.Max.X-client.Max.X), max(0, r.Max.Y-client.Max.Y)))
	}
	overlay.ShowOver(hwnd, r)
	if !placing && ok {
		s.placeGrip(box, client, r)
	}
	return w, placing
}

// moveFrom takes move seq of drag and returns where the box stood when the
// drag began; ok is false for a move to drop (late, or of an ended drag).
func (st *boxDragState) moveFrom(box string, drag, seq int, placing bool, hwnd uintptr) (image.Rectangle, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if drag == st.ended {
		return image.Rectangle{}, false
	}
	if drag != st.drag {
		r, ok := overlay.WindowRect(hwnd)
		if !ok {
			return image.Rectangle{}, false
		}
		st.drag, st.seq, st.start = drag, 0, r
	}
	if seq <= st.seq {
		return image.Rectangle{}, false
	}
	st.seq, st.movedAt = seq, time.Now()
	if !placing {
		st.box = box
	}
	return st.start, true
}

// gripRect is where box's grip goes for the box at r: just above its
// top-right corner, or, with no room above, right of it (left of the box
// when that is off the game too).
func gripRect(client, r image.Rectangle, dpi float64) image.Rectangle {
	w, h, gap := int(gripWidth*dpi), int(gripHeight*dpi), int(gripGap*dpi)
	g := image.Rect(r.Max.X-w, r.Min.Y-gap-h, r.Max.X, r.Min.Y-gap)
	if g.Min.Y >= client.Min.Y {
		return g
	}
	g = image.Rect(r.Max.X+gap, r.Min.Y, r.Max.X+gap+w, r.Min.Y+h)
	if g.Max.X <= client.Max.X {
		return g
	}
	return image.Rect(r.Min.X-gap-w, r.Min.Y, r.Min.X-gap, r.Min.Y+h)
}

// placeGrip shows box's grip by the box at r.
func (s *AppService) placeGrip(box string, client, r image.Rectangle) {
	g := s.grips[box]
	if g == nil {
		return
	}
	dpi := 1.0
	if screen := s.anchorScreen(); screen != nil && screen.ScaleFactor > 0 {
		dpi = float64(screen.ScaleFactor)
	}
	hwnd := uintptr(g.NativeWindow())
	st := &s.boxDrag
	st.mu.Lock()
	if st.gripReady == nil {
		st.gripReady, st.gripOn = map[string]bool{}, map[string]bool{}
	}
	first := !st.gripReady[box]
	st.gripReady[box], st.gripOn[box] = true, true
	st.mu.Unlock()
	if first {
		overlay.Popup(hwnd)
		g.Show()
	}
	// Again after Wails' first showing, which may set the frame back.
	overlay.Popup(hwnd)
	overlay.ShowOver(hwnd, gripRect(client, r, dpi))
	if overlay.ForegroundWindow() == hwnd {
		overlay.FocusGame()
	}
}

// putGripAway takes box's grip off the screen.
func (s *AppService) putGripAway(box string) {
	g := s.grips[box]
	st := &s.boxDrag
	st.mu.Lock()
	on := st.gripOn[box]
	if on {
		st.gripOn[box] = false
	}
	st.mu.Unlock()
	if g != nil && on {
		overlay.ShowOver(uintptr(g.NativeWindow()), gripAway)
	}
}

// syncGrip shows box's grip by the box while the box shows over the game
// (not while it is being placed: the box takes the mouse then), and hides
// it otherwise.
func (s *AppService) syncGrip(box string) {
	g := s.grips[box]
	if g == nil {
		return
	}
	w, placing := s.boxWindow(box)
	if w != nil && !placing && w.IsVisible() {
		if r, ok := overlay.WindowRect(uintptr(w.NativeWindow())); ok {
			if client, ok := overlay.GameClient(); ok {
				s.placeGrip(box, client, r)
				return
			}
		}
	}
	s.putGripAway(box)
}
