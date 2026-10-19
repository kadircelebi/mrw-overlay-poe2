package app

import (
	"image"
	"log"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/lootlog"
	"poe2filter/internal/overlay"
)

// The loot box lies over the game while the inventory scan is on, in maps
// too: what the next map starts from (the last read in the hideout and when
// it was made), the map the player is in and the read it began from, the
// reads back from it, what the last map brought and the session's sum. The
// session counts from the game's start or, later, from when the player
// started it over in the tracker window.

// Where the loot log stands, as the box words it.
const (
	LootUnknown = "unknown" // the game's log has not said where the player is
	LootNoBase  = "noBase"  // in a hideout, no read yet
	LootBased   = "based"   // in a hideout, the next map's base read
	LootInMap   = "inMap"
	LootBack    = "back"    // home from a map, its inventory not read yet
	LootReading = "reading" // reading what the map brought (automatic scan)
)

// LootStatusView is what the loot box shows. InvEx is the worth of the
// inventory last read in a hideout or town (in a map: the read the map
// began from, at BaseAt, Unix ms; 0 when none); Fresh tells it was read on
// this visit to the hideout. Map names the map the player is in or came
// back from (Entered: when they went in). ReadingUntil (Unix ms) is when
// the reads back from the map stop counting; Hotkey is the scan's shortcut
// (the manual rule reads only with it). LastRun is the latest map
// run filed, at LastRunAt.
type LootStatusView struct {
	Mode         string        `json:"mode"`
	Hotkey       string        `json:"hotkey"`
	Placing      bool          `json:"placing"`
	Phase        string        `json:"phase"`
	InvEx        float64       `json:"invEx"`
	BaseAt       int64         `json:"baseAt"`
	Fresh        bool          `json:"fresh"`
	Unsure       int           `json:"unsure"`
	Map          string        `json:"map"`
	Entered      int64         `json:"entered"`
	ReadingUntil int64         `json:"readingUntil"`
	LastRun      *InvScanMap   `json:"lastRun"`
	LastRunAt    int64         `json:"lastRunAt"`
	Session      lootlog.Total `json:"session"`
	SessionStart int64         `json:"sessionStart"`
	Farm         FarmView      `json:"farm"`
	Fields       []string      `json:"fields"`
	DivineEx     float64       `json:"divineEx"`
}

// The loot box's size in window pixels: its width, and per line shown
// (lootBoxLines) besides the frame. The window is made as tall as every
// line needs (lootBoxMaxHeight): a see-through window takes the mouse only
// over the size it was made with, not one SetWindowPos gives it later.
const (
	lootBoxWidth = 250
	lootBoxLine  = 17
	lootBoxFrame = 12
)

var lootBoxMaxHeight = lootBoxFrame + len(overlay.LootBoxFieldKeys)*lootBoxLine

func lootBoxLines(fields []string) int { return max(1, len(fields)) }

// lootSessionStart is when the loot session began: the game's start, or the
// player's restart of it when later.
func (s *AppService) lootSessionStart() (time.Time, bool) {
	start := s.sessionStart()
	farm := s.lootLog.State(s.overlayClient.League()).Farm
	if farm.After(start) {
		return farm, true
	}
	return start, false
}

// GetLootStatus returns what the loot box shows.
func (s *AppService) GetLootStatus() LootStatusView {
	league := s.overlayClient.League()
	mode, _ := s.invScanMode()
	st := s.lootLog.State(league)
	s.overlayMu.RLock()
	hotkey := s.overlaySettings.InventoryScanHotkey
	fields := slices.Clone(s.overlaySettings.LootBoxFields)
	s.overlayMu.RUnlock()
	v := LootStatusView{Mode: mode, Hotkey: hotkey, Fields: fields, Placing: s.lootPlacing.Load(), Phase: LootUnknown}
	snap := s.eng.Prices()
	worth := func(sc *lootlog.Scan) float64 {
		var sum float64
		for name, n := range sc.Items {
			sum += overlay.QuoteCurrency(snap, name).ValueEx * float64(n)
		}
		return sum
	}
	if snap != nil {
		v.DivineEx = overlay.QuoteCurrency(snap, "Divine Orb").DivineEx
	}
	base, fresh := st.Last, st.Fresh
	switch {
	case !st.Known:
	case st.InRest && st.Back:
		v.Phase, v.Map = LootBack, mapName(st.TripArea)
		if until := s.autoScan.readingUntil.Load(); until > time.Now().UnixMilli() {
			v.Phase, v.ReadingUntil = LootReading, until
		}
	case st.InRest && st.Last == nil:
		v.Phase = LootNoBase
	case st.InRest:
		v.Phase = LootBased
	default:
		v.Phase = LootInMap
		base, fresh = nil, false
		if st.TripArea != "" && !st.Back {
			v.Map, v.Entered = mapName(st.TripArea), st.Entered.UnixMilli()
			base, fresh = st.Base, st.TripFresh
		}
	}
	if base != nil {
		v.InvEx, v.BaseAt, v.Fresh, v.Unsure = worth(base), base.At.UnixMilli(), fresh, base.Unsure
	}
	start, _ := s.lootSessionStart()
	v.Farm = s.farmTimes(league, start)
	list := s.lootLog.Maps(league)
	farmFill(&v.Farm, list, start)
	if len(list) > 0 && len(list[0].Runs) > 0 {
		m := list[0]
		r := m.Runs[len(m.Runs)-1]
		v.LastRun = &InvScanMap{Name: mapName(m.AreaID), RunEx: r.ValueEx, MapEx: m.ValueEx, Runs: len(m.Runs), Fresh: r.Fresh, NoBase: r.NoBase || r.Unread, Unsure: r.Unsure}
		v.LastRunAt = r.To.UnixMilli()
	}
	v.Session, v.SessionStart = lootlog.Sum(list, start), start.UnixMilli()
	return v
}

// StartLootSession starts the loot session over now: the maps stay, the
// session's sum counts from here.
func (s *AppService) StartLootSession() LootLogView {
	if err := s.lootLog.StartFarm(s.overlayClient.League(), time.Now()); err != nil {
		log.Printf("loot log: start session: %v", err)
	}
	s.lootChanged()
	return s.GetLootLog()
}

// SetLootBox shows or hides the loot box over the game.
func (s *AppService) SetLootBox(on bool) error {
	next := s.GetOverlaySettings()
	next.LootBoxHidden = !on
	_, err := s.SaveOverlaySettings(next)
	return err
}

func (s *AppService) lootBoxOn() bool {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	st := s.overlaySettings
	return st.Enabled && st.InventoryScan != overlay.ScanOff && !st.LootBoxHidden
}

func (s *AppService) emitLootStatus() {
	if s.app != nil {
		s.app.Event.Emit("loot-status", s.GetLootStatus())
	}
	s.syncLootBox()
}

// syncLootBox shows the loot box over the game while it is the active
// window and the box is on, and hides it otherwise.
func (s *AppService) syncLootBox() {
	defer s.syncGrip("loot")
	w := s.lootBoxWindow
	if w == nil || s.lootPlacing.Load() || s.boxMoving("loot") {
		return
	}
	if !s.lootBoxOn() || !s.gameActive.Load() {
		if w.IsVisible() {
			w.Hide()
		}
		return
	}
	client, ok := overlay.GameClient()
	if !ok {
		return
	}
	showBox(w)
	overlay.ShowOver(uintptr(w.NativeWindow()), s.lootBoxRect(client))
	giveFocusBack(w)
}

// lootBoxRect is the box's place in client for the current settings.
func (s *AppService) lootBoxRect(client image.Rectangle) image.Rectangle {
	s.overlayMu.RLock()
	st := s.overlaySettings
	s.overlayMu.RUnlock()
	dpi := 1.0
	if screen := s.anchorScreen(); screen != nil && screen.ScaleFactor > 0 {
		dpi = float64(screen.ScaleFactor)
	}
	return boxRect(client, st.LootCorner, st.LootX, st.LootY, dpi, lootBoxWidth, lootBoxFrame+lootBoxLines(st.LootBoxFields)*lootBoxLine)
}

// PlaceLootBox starts placing the loot box over the game.
func (s *AppService) PlaceLootBox() string {
	w := s.lootBoxWindow
	if w == nil {
		return PlaceNoGame
	}
	client, ok := overlay.GameClient()
	if !ok {
		return PlaceNoGame
	}
	s.lootPlacing.Store(true)
	w.SetIgnoreMouseEvents(false)
	w.Show()
	overlay.ShowOver(uintptr(w.NativeWindow()), s.lootBoxRect(client))
	s.lootChanged()
	return PlaceOK
}

// FinishLootBoxPlace ends placing; with save, the box keeps the place it
// was dragged to, as a share of the game's client area.
func (s *AppService) FinishLootBoxPlace(save bool) {
	w := s.lootBoxWindow
	if w == nil || !s.lootPlacing.Load() {
		return
	}
	if save {
		r, okWin := overlay.WindowRect(uintptr(w.NativeWindow()))
		client, okGame := overlay.GameClient()
		if okWin && okGame && client.Dx() > 0 && client.Dy() > 0 {
			next := s.GetOverlaySettings()
			next.LootCorner = overlay.EarningsCustom
			next.LootX = float64(r.Min.X-client.Min.X) / float64(client.Dx())
			next.LootY = float64(r.Min.Y-client.Min.Y) / float64(client.Dy())
			if _, err := s.SaveOverlaySettings(next); err != nil {
				log.Printf("loot box: save place: %v", err)
			}
		}
	}
	w.SetIgnoreMouseEvents(true)
	s.lootPlacing.Store(false)
	w.Hide()
	s.lootChanged()
}

// boxWindows are the boxes that lie over the game.
func (s *AppService) boxWindows() []application.Window {
	var out []application.Window
	for _, w := range []application.Window{s.earningsWindow, s.expeditionStripWindow, s.lootBoxWindow} {
		if w != nil {
			out = append(out, w)
		}
	}
	return out
}

// hideBoxesOver hides the boxes (and grips) shown over screen area r,
// which a read of it would take for part of the game; it reports whether
// it hid one.
func (s *AppService) hideBoxesOver(r image.Rectangle) bool {
	hidden := false
	for _, w := range s.boxWindows() {
		if !w.IsVisible() {
			continue
		}
		if box, ok := overlay.WindowRect(uintptr(w.NativeWindow())); ok && box.Overlaps(r) {
			w.Hide()
			hidden = true
		}
	}
	for _, box := range boxNames {
		if g := s.grips[box]; g != nil {
			if gr, ok := overlay.WindowRect(uintptr(g.NativeWindow())); ok && gr.Overlaps(r) {
				s.putGripAway(box)
				hidden = true
			}
		}
	}
	return hidden
}

// syncBoxes puts the boxes back as their settings say.
func (s *AppService) syncBoxes() {
	s.syncEarningsOverlay()
	s.syncExpeditionStrip()
	s.syncLootBox()
}
