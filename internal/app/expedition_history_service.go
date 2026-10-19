package app

import (
	"image"
	"log"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/expeditionhistory"
	"poe2filter/internal/overlay"
)

// The Expedition history keeps the reward chosen on the Runeshape panel in
// each map. The game logs no choice, so it is taken from where the player
// clicks while the price labels show (the labels let clicks through to the
// game; the button is only seen, never pressed or held back). The map comes
// from the game's log: entering another map closes the choice of the one
// before, a trip to the hideout does not.

// ExpeditionEntry is one chosen reward as the tracker window shows it; Map
// is the map's name made readable from its id ("Wayward Isle").
type ExpeditionEntry struct {
	expeditionhistory.Entry
	Map string `json:"map"`
}

// ExpeditionHistoryView is what the tracker window and the overlay show.
// Pending is the latest reward chosen in the map the player is in and Map
// the sum of that map's choices. Session counts from the game's start
// (SessionStart, Unix ms), Today from local midnight, League all of it;
// every choice counts, the current map's included.
type ExpeditionHistoryView struct {
	League       string                  `json:"league"`
	Entries      []ExpeditionEntry       `json:"entries"`
	Pending      *ExpeditionEntry        `json:"pending"`
	Map          expeditionhistory.Total `json:"mapTotal"`
	SessionStart int64                   `json:"sessionStart"`
	Session      expeditionhistory.Total `json:"session"`
	Today        expeditionhistory.Total `json:"today"`
	LeagueTotal  expeditionhistory.Total `json:"leagueTotal"`
	DivineEx     float64                 `json:"divineEx"`
	// Strip shows the pending reward over the game; Placing is set while
	// the player drags its box to its place.
	Strip   bool `json:"strip"`
	Placing bool `json:"placing"`
	// Names lists the current map's choices in the box too.
	Names bool `json:"names"`
}

func (s *AppService) expeditionStripNames() bool {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlaySettings.ExpeditionStripNames
}

func (s *AppService) expeditionStripOn() bool {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlaySettings.Enabled && !s.overlaySettings.ExpeditionStripHidden
}

// GetExpeditionHistory returns the current league's chosen rewards.
func (s *AppService) GetExpeditionHistory() ExpeditionHistoryView {
	league := s.overlayClient.League()
	entries := s.expeditionLog.Entries(league)
	v := ExpeditionHistoryView{League: league, Strip: s.expeditionStripOn(), Placing: s.expeditionPlacing.Load(), Names: s.expeditionStripNames(), Entries: make([]ExpeditionEntry, len(entries))}
	for i, e := range entries {
		v.Entries[i] = ExpeditionEntry{Entry: e, Map: mapName(e.AreaID)}
	}
	if len(v.Entries) > 0 && !v.Entries[0].Done {
		p := v.Entries[0]
		v.Pending = &p
	}
	if snap := s.eng.Prices(); snap != nil {
		v.DivineEx = snap.Rates.DivineEx
	}
	start := s.sessionStart()
	now := time.Now()
	v.SessionStart = start.UnixMilli()
	v.Map = expeditionhistory.Open(entries)
	v.Session = expeditionhistory.Sum(entries, start)
	v.Today = expeditionhistory.Sum(entries, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
	v.LeagueTotal = expeditionhistory.Sum(entries, time.Time{})
	return v
}

// UndoExpeditionChoice takes back the latest choice of the current map.
func (s *AppService) UndoExpeditionChoice() ExpeditionHistoryView {
	if _, ok, err := s.expeditionLog.Undo(s.overlayClient.League()); err != nil {
		log.Printf("expedition history: undo: %v", err)
	} else if ok {
		s.expeditionChanged()
	}
	return s.GetExpeditionHistory()
}

// ResetExpeditionMap forgets every choice of the current map, for choosing
// again from the start.
func (s *AppService) ResetExpeditionMap() ExpeditionHistoryView {
	if n, err := s.expeditionLog.ResetMap(s.overlayClient.League()); err != nil {
		log.Printf("expedition history: reset map: %v", err)
	} else if n > 0 {
		s.expeditionChanged()
	}
	return s.GetExpeditionHistory()
}

// DeleteExpeditionChoice forgets one choice, named by when it was made
// (RFC 3339 as the history shows it).
func (s *AppService) DeleteExpeditionChoice(at string) ExpeditionHistoryView {
	when, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return s.GetExpeditionHistory()
	}
	if ok, err := s.expeditionLog.Delete(s.overlayClient.League(), when); err != nil {
		log.Printf("expedition history: delete: %v", err)
	} else if ok {
		s.expeditionChanged()
	}
	return s.GetExpeditionHistory()
}

// ClearExpeditionHistory forgets the chosen rewards of every league.
func (s *AppService) ClearExpeditionHistory() (ExpeditionHistoryView, error) {
	err := s.expeditionLog.Clear()
	s.expeditionChanged()
	return s.GetExpeditionHistory(), err
}

// SetExpeditionStrip shows or hides the chosen reward over the game.
func (s *AppService) SetExpeditionStrip(on bool) (ExpeditionHistoryView, error) {
	next := s.GetOverlaySettings()
	next.ExpeditionStripHidden = !on
	_, err := s.SaveOverlaySettings(next)
	return s.GetExpeditionHistory(), err
}

func (s *AppService) expeditionChanged() {
	if s.app != nil {
		s.app.Event.Emit("expedition-history", s.GetExpeditionHistory())
	}
	s.syncExpeditionStrip()
}

// The Expedition box's size in window pixels: the current map's and this
// session's chosen rewards, one line each; with the names on, wider and a
// line more for each of the map's choices, up to expeditionStripNameRows.
const (
	expeditionStripWidth     = 190
	expeditionStripHeight    = 46
	expeditionStripNameWidth = 290
	expeditionStripRowHeight = 17
	expeditionStripNameRows  = 8
)

// expeditionStripMaxHeight is the box's height with the most names: the
// window is made that big, as a see-through window takes the mouse only
// over the size it was made with.
var expeditionStripMaxHeight = func() int { _, h := expeditionStripSize(expeditionStripNameRows + 1); return h }()

// expeditionStripSize is the box's size for names of the map's choices
// (0: names off or none chosen yet); past the row limit one line says how
// many more there are.
func expeditionStripSize(names int) (w, h int) {
	if names <= 0 {
		return expeditionStripWidth, expeditionStripHeight
	}
	rows := min(names, expeditionStripNameRows+1)
	return expeditionStripNameWidth, expeditionStripHeight + 4 + rows*expeditionStripRowHeight
}

// syncExpeditionStrip shows the totals of the chosen rewards over the game
// while it is the active window, the box is switched on and the league has a
// choice, and hides it otherwise.
func (s *AppService) syncExpeditionStrip() {
	defer s.syncGrip("expedition")
	w := s.expeditionStripWindow
	if w == nil || s.expeditionPlacing.Load() || s.boxMoving("expedition") {
		return
	}
	any := len(s.expeditionLog.Entries(s.overlayClient.League())) > 0
	if !any || !s.expeditionStripOn() || !s.gameActive.Load() {
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
	overlay.ShowOver(uintptr(w.NativeWindow()), s.expeditionStripRect(client))
	giveFocusBack(w)
}

// showBox shows a box over the game only when it is hidden: Wails shows a
// window activating it, which a visible one need not risk again.
func showBox(box application.Window) {
	if !box.IsVisible() {
		box.Show()
	}
}

// giveFocusBack returns the focus to the game should Windows have given it
// to box, a window made not to take it. Any other window keeps the focus:
// the price check opened by a hotkey closes when it loses it.
func giveFocusBack(box application.Window) {
	if overlay.ForegroundWindow() == uintptr(box.NativeWindow()) {
		overlay.FocusGame()
	}
}

// expeditionStripRect is the box's place in client for the current settings.
func (s *AppService) expeditionStripRect(client image.Rectangle) image.Rectangle {
	s.overlayMu.RLock()
	st := s.overlaySettings
	s.overlayMu.RUnlock()
	dpi := 1.0
	if screen := s.anchorScreen(); screen != nil && screen.ScaleFactor > 0 {
		dpi = float64(screen.ScaleFactor)
	}
	names := 0
	if st.ExpeditionStripNames && !s.expeditionPlacing.Load() {
		names = expeditionhistory.Open(s.expeditionLog.Entries(s.overlayClient.League())).Maps
	}
	w, h := expeditionStripSize(names)
	return boxRect(client, st.ExpeditionCorner, st.ExpeditionX, st.ExpeditionY, dpi, w, h)
}

// PlaceExpeditionStrip starts placing the Expedition box over the game.
func (s *AppService) PlaceExpeditionStrip() string {
	w := s.expeditionStripWindow
	if w == nil {
		return PlaceNoGame
	}
	client, ok := overlay.GameClient()
	if !ok {
		return PlaceNoGame
	}
	s.expeditionPlacing.Store(true)
	w.SetIgnoreMouseEvents(false)
	w.Show()
	overlay.ShowOver(uintptr(w.NativeWindow()), s.expeditionStripRect(client))
	s.expeditionChanged()
	return PlaceOK
}

// FinishExpeditionPlace ends placing; with save, the box keeps the place it
// was dragged to, as a share of the game's client area.
func (s *AppService) FinishExpeditionPlace(save bool) {
	w := s.expeditionStripWindow
	if w == nil || !s.expeditionPlacing.Load() {
		return
	}
	if save {
		r, okWin := overlay.WindowRect(uintptr(w.NativeWindow()))
		client, okGame := overlay.GameClient()
		if okWin && okGame && client.Dx() > 0 && client.Dy() > 0 {
			next := s.GetOverlaySettings()
			next.ExpeditionCorner = overlay.EarningsCustom
			next.ExpeditionX = float64(r.Min.X-client.Min.X) / float64(client.Dx())
			next.ExpeditionY = float64(r.Min.Y-client.Min.Y) / float64(client.Dy())
			if _, err := s.SaveOverlaySettings(next); err != nil {
				log.Printf("expedition box: save place: %v", err)
			}
		}
	}
	w.SetIgnoreMouseEvents(true)
	s.expeditionPlacing.Store(false)
	w.Hide()
	s.expeditionChanged()
}

// expeditionClicked takes a left click at screen pixel x, y while the labels
// of read seq showed: on a reward's row, that reward is chosen.
func (s *AppService) expeditionClicked(seq, x, y int) {
	s.expedition.mu.Lock()
	view, client, area := s.expedition.view, s.expedition.client, s.expedition.area
	s.expedition.mu.Unlock()
	if view.Seq != seq || area.ID == "" {
		return
	}
	i := pickExpeditionRow(view, client, x, y)
	if i < 0 {
		return
	}
	row := view.Rows[i]
	e := expeditionhistory.Entry{
		At: time.Now(), AreaID: area.ID, AreaLevel: area.Level, Seed: area.Seed,
		Name: row.Name, Display: row.Display, Text: row.Text, Count: row.Count, CountKnown: row.CountKnown, ValueEx: row.ValueEx,
	}
	if err := s.expeditionLog.Choose(s.overlayClient.League(), e); err != nil {
		log.Printf("expedition history: %v", err)
	}
	s.expeditionChanged()
}

// expeditionAreaEntered follows the game into an area: another map closes
// the pending choice.
func (s *AppService) expeditionAreaEntered(a overlay.Area) {
	changed, err := s.expeditionLog.Entered(s.overlayClient.League(), a.ID, a.Seed, a.Rest())
	if err != nil {
		log.Printf("expedition history: %v", err)
	}
	if changed {
		s.expeditionChanged()
	}
}

// pickExpeditionRow returns the row a click at screen pixel x, y fell on,
// or -1. Rows are bands of the panel's height split halfway between their
// lines; across, the click must be left of the panel's right edge and not
// far left of the reward texts (the rune icons sit there).
func pickExpeditionRow(view ExpeditionView, client image.Rectangle, x, y int) int {
	if len(view.Rows) == 0 || client.Empty() {
		return -1
	}
	cx, cy := float64(x-client.Min.X), float64(y-client.Min.Y)
	type band struct {
		i      int
		center float64
	}
	bands := make([]band, len(view.Rows))
	left, height := view.Rows[0].X, 0.0
	for i, r := range view.Rows {
		bands[i] = band{i, r.Y + r.H/2}
		left = min(left, r.X)
		height += r.H
	}
	height /= float64(len(view.Rows))
	if cx > view.PanelRight+height || cx < left-12*height {
		return -1
	}
	slices.SortFunc(bands, func(a, b band) int {
		switch {
		case a.center < b.center:
			return -1
		case a.center > b.center:
			return 1
		}
		return 0
	})
	// A lone row reaches a line height above and below its text.
	gap := 2 * height
	if len(bands) > 1 {
		gap = (bands[len(bands)-1].center - bands[0].center) / float64(len(bands)-1)
	}
	for k, b := range bands {
		top, bottom := b.center-gap/2, b.center+gap/2
		if k > 0 {
			top = (bands[k-1].center + b.center) / 2
		}
		if k < len(bands)-1 {
			bottom = (b.center + bands[k+1].center) / 2
		}
		if cy >= top && cy < bottom {
			return b.i
		}
	}
	return -1
}

// mapName makes an area id readable: "MapWaywardIsle" → "Wayward Isle".
func mapName(id string) string {
	id = strings.TrimPrefix(id, "Map")
	var b strings.Builder
	prev := rune(0)
	for _, r := range id {
		if r == '_' {
			r = ' '
		} else if unicode.IsUpper(r) && prev != 0 && prev != ' ' && !unicode.IsUpper(prev) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return strings.TrimSpace(b.String())
}
