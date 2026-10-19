package app

import (
	"context"
	"image"
	"log"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"poe2filter/internal/invscan"
	"poe2filter/internal/lootlog"
	"poe2filter/internal/overlay"
)

// invScanShowFor is how long the inventory's worth stays over the game; a
// click anywhere, Escape or another press of the shortcut hides it sooner.
const invScanShowFor = 30 * time.Second

// invScanNoticeFor is how long a notice ("open the inventory") stays up.
const invScanNoticeFor = 3 * time.Second

// invScanSettle is how long the game takes to drop an item's tooltip once
// the pointer leaves it, before the screen is read.
const invScanSettle = 90 * time.Millisecond

// InvScanRow is one item of the inventory as the worth card lists it: how
// many there are in all its stacks, one's worth and theirs (Exalted), and
// the loot filter's colours for that worth (as in the Expedition labels).
type InvScanRow struct {
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	EachEx  float64 `json:"eachEx"`
	TotalEx float64 `json:"totalEx"`
	Bg      string  `json:"bg"`
	Color   string  `json:"color"`
	Border  string  `json:"border"`
}

// InvScanView is what the worth card shows. Right and Top place it in the
// game's client area (physical pixels): its right edge left of the
// inventory panel. Off is the filled cells holding items not on the price
// list's exchange (gear, tablets, waystones); Unsure the cells whose item
// or count could not be told for sure (left out of the total, or counted
// as read); Unpriced the listed items the price list has no worth for.
// Message is "loading" (icons still downloading), "closed" (no inventory
// on the screen), "button" (a mouse button held), "error", or "".
type InvScanView struct {
	Rows     []InvScanRow `json:"rows"`
	TotalEx  float64      `json:"totalEx"`
	DivineEx float64      `json:"divineEx"`
	Off      int          `json:"off"`
	Unsure   int          `json:"unsure"`
	// Doubts are the cells counted in Unsure, where they are and why.
	Doubts   []lootlog.Doubt `json:"doubts"`
	Unpriced int             `json:"unpriced"`
	Message  string          `json:"message"`
	// Map is the map this read closed a run of (nil: none); Base tells the
	// read was kept as the next map's base (made in a hideout or town);
	// InMap that it was made in a map, and kept for nothing.
	Map   *InvScanMap `json:"map"`
	Base  bool        `json:"base"`
	InMap bool        `json:"inMap"`
	Right float64     `json:"right"`
	Top   float64     `json:"top"`
	Scale float64     `json:"scale"`
	Seq   int         `json:"seq"`
}

type invScanState struct {
	mu       sync.Mutex
	icons    []invscan.Icon
	loading  bool
	matchers map[int]*invscan.Matcher
	view     InvScanView
	timer    *time.Timer
	busy     bool
	// doubtsKept are the doubtful cells whose pictures the last read kept.
	doubtsKept map[string]bool
}

func (s *AppService) invScanMode() (string, bool) {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlaySettings.InventoryScan, s.overlaySettings.Enabled
}

// GetInvScanView returns the card last read, for a window that opens after
// the read.
func (s *AppService) GetInvScanView() InvScanView {
	s.invScan.mu.Lock()
	defer s.invScan.mu.Unlock()
	return s.invScan.view
}

// loadInvScanIcons downloads the exchange items' icons (once; later from
// disk) when the inventory scan is on, in the background.
func (s *AppService) loadInvScanIcons() {
	mode, enabled := s.invScanMode()
	if !enabled || mode == overlay.ScanOff {
		return
	}
	s.invScan.mu.Lock()
	if s.invScan.loading || len(s.invScan.icons) > 0 {
		s.invScan.mu.Unlock()
		return
	}
	s.invScan.loading = true
	s.invScan.mu.Unlock()
	go func() {
		defer func() {
			s.invScan.mu.Lock()
			s.invScan.loading = false
			s.invScan.mu.Unlock()
		}()
		ctx := context.Background()
		catalog, err := s.overlayCatalog.Load(ctx)
		if err != nil {
			log.Printf("inventory scan: catalog: %v", err)
			return
		}
		var sources []invscan.Source
		for _, c := range catalog.Currencies {
			if invscan.InScope(c.Group) {
				sources = append(sources, invscan.Source{Entry: invscan.Entry{ID: c.ID, Name: c.Text, Group: c.Group}, URL: c.Image})
			}
		}
		cache := invscan.IconCache{Dir: filepath.Join(s.meta.DataDir, "data", "icons"), Workers: 2, Pause: 50 * time.Millisecond}
		icons, err := cache.Load(ctx, sources)
		if err != nil {
			log.Printf("inventory scan: icons: %v", err)
		}
		s.invScan.mu.Lock()
		s.invScan.icons = icons
		s.invScan.matchers = map[int]*invscan.Matcher{}
		s.invScan.mu.Unlock()
	}()
}

// matcher is the icon matcher for cells of the given side.
func (s *AppService) invScanMatcher(cell int) *invscan.Matcher {
	s.invScan.mu.Lock()
	defer s.invScan.mu.Unlock()
	if len(s.invScan.icons) == 0 {
		return nil
	}
	if m, ok := s.invScan.matchers[cell]; ok {
		return m
	}
	m := invscan.NewMatcher(s.invScan.icons, cell)
	s.invScan.matchers[cell] = m
	return m
}

// captureInventory reads the inventory off the screen once and shows its
// currency's worth over the game; pressed again while that shows, it hides
// it. The pointer is moved off the inventory for the moment the screen is
// read (an item's tooltip would hide the cells behind it) and put back.
func (s *AppService) captureInventory() {
	mode, enabled := s.invScanMode()
	if !enabled || mode == overlay.ScanOff || s.invScanWindow == nil {
		return
	}
	if s.invScanWindow.IsVisible() {
		s.invScanWindow.Hide()
		return
	}
	if !overlay.IsGameWindow(overlay.ForegroundWindow()) {
		return
	}
	s.invScan.mu.Lock()
	if s.invScan.busy {
		s.invScan.mu.Unlock()
		return
	}
	s.invScan.busy = true
	s.invScan.mu.Unlock()
	defer func() {
		s.invScan.mu.Lock()
		s.invScan.busy = false
		s.invScan.mu.Unlock()
	}()

	client, ok := overlay.GameClient()
	if !ok {
		return
	}
	guess := invscan.InventoryGuess(client.Dx(), client.Dy())
	view := InvScanView{Right: guess.X - 0.05*float64(client.Dy()), Top: 0.1 * float64(client.Dy()), Scale: float64(client.Dy()) / 2160}
	if overlay.MouseButtonDown() {
		view.Message = "button"
		s.showInvScan(view, client)
		return
	}
	m := s.invScanMatcher(int(guess.Cell + 0.5))
	if m == nil {
		s.loadInvScanIcons()
		view.Message = "loading"
		s.showInvScan(view, client)
		return
	}

	img, err := s.grabInventory(client)
	if err != nil {
		log.Printf("inventory scan: screen: %v", err)
		view.Message = "error"
		s.showInvScan(view, client)
		return
	}
	pic := invscan.FromImage(img, img.Bounds())
	g := invscan.Align(pic, guess)
	if !invscan.Shown(pic, g) {
		view.Message = "closed"
		s.showInvScan(view, client)
		return
	}
	if int(g.Cell+0.5) != int(guess.Cell+0.5) {
		if mm := s.invScanMatcher(int(g.Cell + 0.5)); mm != nil {
			m = mm
		}
	}
	s.priceInventory(&view, m.ReadGrid(pic, g))
	s.keepDoubtPictures(pic, g, view.Doubts)
	s.recordInventory(&view)
	s.showInvScan(view, client)
}

// grabInventory reads the game's client area with the pointer moved to the
// inventory's title (no item there, so no tooltip), and the boxes the app
// lays over the game hidden for that moment.
func (s *AppService) grabInventory(client image.Rectangle) (*image.RGBA, error) {
	x, y, ok := overlay.CursorPosition()
	hidden := s.hideBoxesForRead()
	if ok {
		h := float64(client.Dy())
		overlay.MoveCursor(client.Max.X-int(0.32*h), client.Min.Y+int(0.065*h))
	}
	time.Sleep(invScanSettle)
	img, _, err := overlay.CaptureGameClient()
	if ok {
		overlay.MoveCursor(x, y)
	}
	if hidden {
		go s.syncBoxes()
	}
	return img, err
}

// hideBoxesForRead hides the boxes the app lays over the game, which
// would be read with it; it reports whether one was shown.
func (s *AppService) hideBoxesForRead() bool {
	hidden := false
	for _, w := range s.boxWindows() {
		if w.IsVisible() {
			w.Hide()
			hidden = true
		}
	}
	for _, box := range boxNames {
		s.putGripAway(box)
	}
	return hidden
}

// priceInventory adds up the read cells by item: an icon shared by several
// items (an orb's tiers, a flux's levels) is told by the number written on
// the cell (invscan.Pick); worth comes from the loot filter's price list.
func (s *AppService) priceInventory(view *InvScanView, cells []invscan.Cell) {
	snap := s.eng.Prices()
	view.DivineEx = overlay.QuoteCurrency(snap, "Divine Orb").DivineEx
	style := s.expeditionStyler(snap)
	rows := map[string]*InvScanRow{}
	for _, c := range cells {
		if c.Icon < 0 {
			view.Off++
			continue
		}
		e, ok := invscan.Pick(c.Entries, c.Marks)
		if !ok {
			view.Unsure++
			view.Doubts = append(view.Doubts, lootlog.Doubt{Row: c.Row + 1, Col: c.Col + 1, Name: baseName(c.Entries), Count: c.Stack, Why: "item"})
			continue
		}
		if c.Unsure {
			// The item is sure, its count may be short of a figure.
			view.Unsure++
			view.Doubts = append(view.Doubts, lootlog.Doubt{Row: c.Row + 1, Col: c.Col + 1, Name: e.Name, Count: c.Stack, Why: "count"})
		}
		r := rows[e.Name]
		if r == nil {
			r = &InvScanRow{Name: e.Name}
			if q := overlay.QuoteCurrency(snap, e.Name); q.Found {
				r.EachEx = q.ValueEx
			}
			rows[e.Name] = r
		}
		r.Count += c.Stack
	}
	for _, r := range rows {
		r.TotalEx = r.EachEx * float64(r.Count)
		if r.EachEx <= 0 {
			view.Unpriced++
		}
		p := ExpeditionPrice{Name: r.Name, ValueEx: r.TotalEx}
		style(&p)
		r.Bg, r.Color, r.Border = p.Bg, p.Color, p.Border
		view.TotalEx += r.TotalEx
		view.Rows = append(view.Rows, *r)
	}
	sort.Slice(view.Rows, func(i, j int) bool {
		if view.Rows[i].TotalEx != view.Rows[j].TotalEx {
			return view.Rows[i].TotalEx > view.Rows[j].TotalEx
		}
		return view.Rows[i].Name < view.Rows[j].Name
	})
}

// baseName names the items sharing an icon by the shortest of their names
// (the plain orb, not its Greater and Perfect tiers).
func baseName(entries []invscan.Entry) string {
	name := ""
	for _, e := range entries {
		if name == "" || len(e.Name) < len(name) {
			name = e.Name
		}
	}
	return name
}

// showInvScan shows view over the game's client area until a click, Escape,
// another press or the time runs out.
func (s *AppService) showInvScan(view InvScanView, client image.Rectangle) {
	s.invScan.mu.Lock()
	view.Seq = s.invScan.view.Seq + 1
	s.invScan.view = view
	if s.invScan.timer != nil {
		s.invScan.timer.Stop()
	}
	showFor := invScanShowFor
	if view.Message != "" {
		showFor = invScanNoticeFor
	}
	seq := view.Seq
	s.invScan.timer = time.AfterFunc(showFor, func() { s.hideInvScan(seq) })
	s.invScan.mu.Unlock()
	s.app.Event.Emit("invscan-view", view)
	s.invScanWindow.Show()
	overlay.ShowOver(uintptr(s.invScanWindow.NativeWindow()), client)
	giveFocusBack(s.invScanWindow)
	go s.hideInvScanOnClick(seq)
}

func (s *AppService) hideInvScan(seq int) {
	s.invScan.mu.Lock()
	current := s.invScan.view.Seq == seq
	s.invScan.mu.Unlock()
	if current && s.invScanWindow.IsVisible() {
		s.invScanWindow.Hide()
	}
}

// hideInvScanOnClick hides card seq at the next click or Escape; a button
// already down when it appears counts only once released.
func (s *AppService) hideInvScanOnClick(seq int) {
	held := overlay.ClickOrEscape()
	shown, start := false, time.Now()
	for range time.Tick(30 * time.Millisecond) {
		s.invScan.mu.Lock()
		current := s.invScan.view.Seq == seq
		s.invScan.mu.Unlock()
		visible := s.invScanWindow.IsVisible()
		shown = shown || visible
		if !current || !visible && (shown || time.Since(start) > time.Second) {
			return
		}
		down := overlay.ClickOrEscape()
		if down && !held {
			s.hideInvScan(seq)
			return
		}
		held = down
	}
}
