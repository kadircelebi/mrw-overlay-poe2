package app

import (
	"context"
	"image"
	"log"
	"maps"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"poe2filter/internal/invscan"
	"poe2filter/internal/lootlog"
	"poe2filter/internal/overlay"
)

// The automatic inventory scan (overlay.ScanAuto) watches, in a hideout or
// town only, whether the inventory is open, and reads it while it is: the
// game can only change what the inventory holds while the panel is open
// (stash, vendor, trade, using an item all open it), and entering a map
// through the atlas opens it too (the waystone goes from it into the
// slot). So:
//
//   - a map's base is the last read made before leaving for it, kept once
//     two reads in a row agree, with the pointer off the inventory panel
//     (an item's tooltip would hide cells);
//   - back from the map, what it brought is read from the first seconds the
//     inventory shows (afterWindow): each item at the most of it any read
//     saw, since putting things in the stash only lowers the counts and a
//     tooltip only hides cells.
//
// Gold (when on) is read with the inventory, every few seconds. Experience
// (when on) takes the pointer on the bar for a moment, so it is read only
// when it can have moved: once back from each map, or when no read has it
// yet, and not while a mouse button is down.
//
// Nothing is read in a map.

const (
	autoPollSlow = time.Second
	// autoPollFast is the pace once back from a map until the inventory
	// first shows (for autoFastFor): the first frames tell what came back.
	autoPollFast = 150 * time.Millisecond
	// autoPollOpen is the pace while the inventory is open at home: a base
	// takes three frames (the panel still, then the same read twice).
	autoPollOpen = 300 * time.Millisecond
	autoFastFor  = 90 * time.Second
	// afterWindow is how long after the inventory first shows on the way
	// back its reads count towards what the map brought.
	afterWindow = 15 * time.Second
	// goldEvery is how often gold is read with the inventory open at home.
	goldEvery = 2 * time.Second
	// expReadFor is how long the pointer stays on the experience bar at most,
	// waiting for its tooltip; expMaxTries how many times it goes there
	// between one map and the next.
	expReadFor  = 300 * time.Millisecond
	expMaxTries = 3
)

type autoScanState struct {
	mu    sync.Mutex
	cache invscan.GridCache
	cell  int
	// last is the previous read (base reads need two that agree) and kept
	// the last one filed as a base.
	last, kept map[string]int
	lastUnsure int
	// after collects the reads back from a map until afterUntil.
	after       map[string]int
	afterUnsure int
	// afterDoubts are the doubts of the read back with the fewest of them
	// (afterUnsure; −1 before the first).
	afterDoubts []lootlog.Doubt
	afterUntil  time.Time
	// afterRead is the experience and gold read back from a map.
	afterRead readout
	// gold is the gold last read at home, at goldAt.
	gold   *int64
	goldAt time.Time
	// expTries counts the pointer's trips to the experience bar since the
	// last map was entered.
	expTries atomic.Int32
	// readingUntil is afterUntil in Unix ms, for the loot box (read
	// without the lock).
	readingUntil atomic.Int64
	// grid is where the inventory's grid was found in the last frame
	// (gridSeen: one was found): a frame is read only once the grid stands
	// where it stood in the one before, as the panel slides in when it
	// opens and a frame of it moving cuts every cell off its numbers.
	grid     invscan.Grid
	gridSeen bool
}

// settled tells whether g stands where the last frame's grid stood, and
// keeps it for the next.
func (st *autoScanState) settled(g invscan.Grid) bool {
	same := st.gridSeen && math.Abs(g.X-st.grid.X) <= 2 && math.Abs(g.Y-st.grid.Y) <= 2 && math.Abs(g.Cell-st.grid.Cell) <= 1
	st.grid, st.gridSeen = g, true
	return same
}

func (s *AppService) autoScanLoop(ctx context.Context) {
	for {
		wait := s.autoScanTick()
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// autoScanTick looks once and says when to look again.
func (s *AppService) autoScanTick() time.Duration {
	st := &s.autoScan
	st.mu.Lock()
	defer st.mu.Unlock()
	mode, enabled := s.invScanMode()
	if !enabled || mode != overlay.ScanAuto {
		st.reset()
		return autoPollSlow
	}
	s.closeAfterLocked(false)
	if !overlay.IsGameWindow(overlay.ForegroundWindow()) {
		return autoPollSlow
	}
	s.placeLoot()
	if !s.lootLog.InRest() {
		st.last = nil
		return autoPollSlow
	}
	wait := autoPollSlow
	_, since, waiting := s.lootLog.Waiting(s.overlayClient.League())
	if (waiting && time.Since(since) < autoFastFor) || !st.afterUntil.IsZero() {
		wait = autoPollFast
	}
	s.invScan.mu.Lock()
	busy := s.invScan.busy || len(s.invScan.icons) == 0
	s.invScan.mu.Unlock()
	if busy {
		s.loadInvScanIcons()
		return wait
	}
	client, ok := overlay.GameClient()
	if !ok {
		return wait
	}
	guess := invscan.InventoryGuess(client.Dx(), client.Dy())
	margin := guess.Cell * 0.3
	area := image.Rect(
		client.Min.X+int(guess.X-margin), client.Min.Y+int(guess.Y-margin),
		client.Min.X+int(guess.X+float64(guess.Cols)*guess.Cell+margin), client.Min.Y+int(guess.Y+float64(guess.Rows)*guess.Cell+margin))
	cx, cy, _ := overlay.CursorPosition()
	panelLeft := client.Min.X + int(guess.X-0.05*float64(client.Dy()))
	overPanel := image.Pt(cx, cy).In(image.Rect(panelLeft, client.Min.Y, client.Max.X, client.Max.Y))
	if overPanel && st.afterUntil.IsZero() && !waiting {
		// Base reads keep to a panel without tooltips.
		return wait
	}
	hidden := s.hideBoxesOver(area)
	img, err := overlay.CaptureScreenRect(area)
	if hidden {
		go s.syncBoxes()
	}
	if err != nil {
		return wait
	}
	pic := invscan.FromImage(img, img.Bounds())
	local := guess
	local.X, local.Y = margin, margin
	g := invscan.Align(pic, local)
	if !invscan.Shown(pic, g) {
		st.last, st.gridSeen = nil, false
		return wait
	}
	wait = min(wait, autoPollOpen)
	if !st.settled(g) {
		return wait
	}
	cell := int(g.Cell + 0.5)
	m := s.invScanMatcher(cell)
	if m == nil {
		return wait
	}
	if cell != st.cell {
		st.cache, st.cell = invscan.GridCache{}, cell
	}
	var view InvScanView
	s.priceInventory(&view, m.ReadGridCached(pic, g, &st.cache))
	s.keepDoubtPictures(pic, g, view.Doubts)
	items := readItems(&view)
	readExp, readGold := s.readoutsOn()
	// The grid in screen pixels, for the gold box under it.
	onScreen := g
	onScreen.X += float64(area.Min.X)
	onScreen.Y += float64(area.Min.Y)

	if waiting || !st.afterUntil.IsZero() {
		// Back from a map: the most of each item seen in the first seconds.
		if st.afterUntil.IsZero() {
			st.after, st.afterUnsure, st.afterDoubts = map[string]int{}, -1, nil
			st.afterRead = readout{}
			st.afterUntil = time.Now().Add(afterWindow)
			st.readingUntil.Store(st.afterUntil.UnixMilli())
			go s.emitLootStatus()
		}
		for name, n := range items {
			st.after[name] = max(st.after[name], n)
		}
		if st.afterUnsure < 0 || view.Unsure < st.afterUnsure {
			st.afterUnsure, st.afterDoubts = view.Unsure, view.Doubts
		}
		// The gold of the first read, before any of it is spent.
		if readGold && st.afterRead.gold == nil {
			st.afterRead.gold = s.readGoldBox(onScreen, client.Dy())
		}
		if readExp && st.afterRead.exp == nil {
			e := s.readExpBar(client)
			st.afterRead.exp, st.afterRead.next = e.exp, e.next
		}
		s.closeAfterLocked(false)
		return wait
	}

	league := s.overlayClient.League()
	if readGold && time.Since(st.goldAt) >= goldEvery {
		st.goldAt = time.Now()
		if gold := s.readGoldBox(onScreen, client.Dy()); gold != nil {
			st.gold = gold
		}
	}
	have := s.lootLog.State(league).Last != nil
	if st.newBase(items, view.Unsure, overPanel, have) {
		s.keepRead(items, view.Unsure, view.Doubts, readout{gold: st.gold})
		have = true
	}
	var e readout
	if readExp && have && s.lootLog.NeedExp(league) {
		e = s.readExpBar(client)
	}
	if e.exp != nil || st.gold != nil {
		if err := s.lootLog.SetReadout(league, e.exp, e.next, st.gold); err != nil {
			log.Printf("loot log: %v", err)
		}
	}
	return wait
}

// readGoldBox reads the gold under grid g (screen pixels), from a game client
// height pixels high.
func (s *AppService) readGoldBox(g invscan.Grid, height int) *int64 {
	r := invscan.GoldRect(g)
	hidden := s.hideBoxesOver(r)
	img, err := overlay.CaptureScreenRect(r)
	if hidden {
		go s.syncBoxes()
	}
	if err != nil {
		return nil
	}
	return goldOf(img, img.Bounds(), height)
}

// readExpBar moves the pointer onto the experience bar until its tooltip
// reads or expReadFor is over, and puts it back; it does nothing while a
// mouse button is down or once expMaxTries is spent. The game sees the
// pointer move as when the player moves the mouse: no click, no key.
func (s *AppService) readExpBar(client image.Rectangle) readout {
	st := &s.autoScan
	x, y, ok := overlay.CursorPosition()
	if !ok || overlay.MouseButtonDown() || st.expTries.Load() >= expMaxTries {
		return readout{}
	}
	st.expTries.Add(1)
	w, h := client.Dx(), client.Dy()
	r := invscan.ExpRect(w, h).Add(client.Min)
	hidden := s.hideBoxesOver(r)
	p := invscan.ExpPoint(w, h).Add(client.Min)
	overlay.MoveCursor(p.X, p.Y)
	defer func() {
		overlay.MoveCursor(x, y)
		if hidden {
			go s.syncBoxes()
		}
	}()
	for start := time.Now(); time.Since(start) < expReadFor; {
		img, err := overlay.CaptureScreenRect(r)
		if err != nil {
			return readout{}
		}
		if e, ok := overlay.ReadExperience(img, img.Bounds(), h); ok {
			st.expTries.Store(0)
			return readout{exp: &e.Current, next: &e.Next}
		}
	}
	log.Print("inventory scan: experience not read")
	return readout{}
}

// newBase takes a read in a hideout or town and tells whether it is the
// next map's new base: the same as the read before (an empty inventory
// too: a read is never nil, unlike "none yet"), made with the pointer off
// the panel, and not the base already kept, while the loot log still has
// one (have: the player may have cleared it).
func (st *autoScanState) newBase(items map[string]int, unsure int, overPanel, have bool) bool {
	stable := st.last != nil && maps.Equal(items, st.last) && unsure == st.lastUnsure
	st.last, st.lastUnsure = items, unsure
	if !stable || overPanel || have && st.kept != nil && maps.Equal(items, st.kept) {
		return false
	}
	st.kept = items
	return true
}

func (st *autoScanState) reset() {
	st.cache, st.cell = invscan.GridCache{}, 0
	st.last, st.kept, st.after = nil, nil, nil
	st.lastUnsure, st.afterUnsure, st.afterUntil = 0, 0, time.Time{}
	st.afterRead, st.gold, st.goldAt = readout{}, nil, time.Time{}
	st.gridSeen = false
	st.readingUntil.Store(0)
}

// closeAfterIfDue files the reads made back from a map once their window
// is over (or now, the player leaving for the next map); the loot box shows
// what the map brought.
func (s *AppService) closeAfterIfDue(now bool) {
	s.autoScan.mu.Lock()
	defer s.autoScan.mu.Unlock()
	s.closeAfterLocked(now)
}

func (s *AppService) closeAfterLocked(now bool) {
	st := &s.autoScan
	if st.afterUntil.IsZero() || (!now && time.Now().Before(st.afterUntil)) {
		return
	}
	items, unsure, doubts, read := st.after, max(0, st.afterUnsure), st.afterDoubts, st.afterRead
	st.after, st.afterUntil, st.afterDoubts, st.afterRead = nil, time.Time{}, nil, readout{}
	st.readingUntil.Store(0)
	st.last, st.kept = nil, items
	st.gold = read.gold
	if m := s.keepRead(items, unsure, doubts, read); m != nil {
		log.Printf("loot log: %s brought %.1f ex", m.Name, m.RunEx)
	}
}
