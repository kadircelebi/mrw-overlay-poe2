package app

import (
	"context"
	"errors"
	"image"
	"log"
	"sync"
	"time"

	"poe2filter/internal/overlay"
	"poe2filter/internal/trade"
	"poe2filter/internal/tradehistory"
)

// The trade history follows the account's merchant sales. The trade site
// answers only signed in and allows 15 requests in three hours per account
// (shared with the player's own visits to the site's history page; the
// client uses at most 80% of it, 12), so it is asked:
//
//   - shortly after start;
//   - after Ange speaks in the game's log (a sale or a listing change), at
//     most once per tradeHistoryAngeGap: 3 h / 15 min = 12 requests at most.
//     A line inside the gap, or one met by a full quota, is put off, not
//     dropped, so a sale shows within about tradeHistoryAngeGap;
//   - as a fallback, when nothing was asked for tradeHistoryFallback (sales
//     made while the game is closed write nothing to its log).
const (
	tradeHistoryFirst     = 20 * time.Second
	tradeHistoryAfterAnge = 45 * time.Second
	tradeHistoryAngeGap   = 15 * time.Minute
	tradeHistoryFallback  = 60 * time.Minute
	tradeHistoryCheck     = 5 * time.Minute
)

// Trade history states shown by the window and the overlay.
const (
	HistoryOff       = "off"       // following is switched off
	HistorySignedOut = "signedOut" // no pathofexile.com session, or it expired
	HistoryWait      = "wait"      // the quota has no room; NextInSec says when
	HistoryError     = "error"     // the site did not answer
	HistoryOK        = "ok"
)

type tradeHistoryState struct {
	mu       sync.Mutex
	store    *tradehistory.Store
	appStart time.Time
	status   string
	err      string
	fetching bool
	// lastTry is when a request last went out; lastAnge when a merchant
	// line last led to one.
	lastTry  time.Time
	lastAnge time.Time
	kick     chan struct{}
	// ange is the delayed request after a merchant line.
	ange *time.Timer
}

// TradeSale is one sale with its worth at today's rates.
type TradeSale struct {
	trade.HistorySale
	// ValueEx is 0 when the currency has no rate.
	ValueEx float64 `json:"valueEx"`
}

// TradeHistoryView is what the tracker window and the earnings overlay show.
// Session counts from the game's start (SessionStart, Unix ms), Today from
// local midnight, League from the first sale kept for the league.
type TradeHistoryView struct {
	Enabled       bool               `json:"enabled"`
	SignedIn      bool               `json:"signedIn"`
	League        string             `json:"league"`
	Status        string             `json:"status"`
	Error         string             `json:"error,omitempty"`
	Fetching      bool               `json:"fetching"`
	FetchedAt     int64              `json:"fetchedAt"`
	NextInSec     int                `json:"nextInSec"`
	SessionStart  int64              `json:"sessionStart"`
	Session       tradehistory.Total `json:"session"`
	Today         tradehistory.Total `json:"today"`
	LeagueTotal   tradehistory.Total `json:"leagueTotal"`
	DivineEx      float64            `json:"divineEx"`
	Sales         []TradeSale        `json:"sales"`
	Overlay       bool               `json:"overlay"`
	OverlayCorner string             `json:"overlayCorner"`
	// Placing is set while the player drags the overlay to its place.
	Placing bool `json:"placing"`
}

func (s *AppService) tradeHistoryOn() (history, overlayOn bool, corner string) {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	st := s.overlaySettings
	return st.TradeHistory, st.TradeHistory && st.EarningsOverlay, st.EarningsCorner
}

// sessionStart is when this game session began: the game's own start from
// its log, or the app's start when the log has not been read yet.
func (s *AppService) sessionStart() time.Time {
	if at := s.whispers.SessionStart(); !at.IsZero() {
		return at
	}
	return s.history.appStart
}

// GetTradeHistory returns the kept sales of the current league and their
// totals.
func (s *AppService) GetTradeHistory() TradeHistoryView {
	on, overlayOn, corner := s.tradeHistoryOn()
	league := s.overlayClient.League()
	h := &s.history
	h.mu.Lock()
	status, errText, fetching := h.status, h.err, h.fetching
	h.mu.Unlock()
	v := TradeHistoryView{
		Enabled: on, SignedIn: s.overlayClient.SignedIn(), League: league,
		Status: status, Error: errText, Fetching: fetching,
		Overlay: overlayOn, OverlayCorner: corner, Placing: s.earningsPlacing.Load(),
	}
	if !on {
		v.Status = HistoryOff
	} else if !v.SignedIn {
		v.Status = HistorySignedOut
	}
	if at := h.store.Fetched(league); !at.IsZero() {
		v.FetchedAt = at.UnixMilli()
	}
	v.NextInSec = int((s.overlayClient.HistoryNextIn() + time.Second - 1) / time.Second)

	snap := s.eng.Prices()
	toEx := func(amount float64, currency string) (float64, bool) {
		if snap == nil {
			return 0, false
		}
		return snap.ToEx(amount, currency)
	}
	if snap != nil {
		v.DivineEx = snap.Rates.DivineEx
	}
	sales := h.store.Sales(league)
	v.Sales = make([]TradeSale, len(sales))
	for i, sale := range sales {
		v.Sales[i].HistorySale = sale
		v.Sales[i].ValueEx, _ = toEx(sale.Amount, sale.Currency)
	}
	start := s.sessionStart()
	v.SessionStart = start.UnixMilli()
	now := time.Now()
	v.Session = tradehistory.Sum(sales, start, toEx)
	v.Today = tradehistory.Sum(sales, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), toEx)
	v.LeagueTotal = tradehistory.Sum(sales, time.Time{}, toEx)
	return v
}

// RefreshTradeHistory asks the trade site now, when the quota allows.
func (s *AppService) RefreshTradeHistory() TradeHistoryView {
	s.fetchTradeHistory()
	return s.GetTradeHistory()
}

// ClearTradeHistory forgets the kept sales of every league.
func (s *AppService) ClearTradeHistory() (TradeHistoryView, error) {
	err := s.history.store.Clear()
	view := s.GetTradeHistory()
	s.emitTradeHistory(view)
	return view, err
}

// fetchTradeHistory runs one request and merges the answer.
func (s *AppService) fetchTradeHistory() {
	if on, _, _ := s.tradeHistoryOn(); !on {
		return
	}
	h := &s.history
	h.mu.Lock()
	if h.fetching {
		h.mu.Unlock()
		return
	}
	h.fetching = true
	h.mu.Unlock()
	s.emitTradeHistory(s.GetTradeHistory())

	league := s.overlayClient.League()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	sales, err := s.overlayClient.TradeHistory(ctx, league)
	var waited *trade.HistoryWaitError
	if !errors.As(err, &waited) && !errors.Is(err, trade.ErrSignedOut) {
		h.mu.Lock()
		h.lastTry = time.Now()
		h.mu.Unlock()
	}
	cancel()
	status, errText := HistoryOK, ""
	var wait *trade.HistoryWaitError
	switch {
	case errors.Is(err, trade.ErrSignedOut):
		status = HistorySignedOut
	case errors.As(err, &wait):
		status = HistoryWait
	case err != nil:
		status, errText = HistoryError, err.Error()
		log.Printf("trade history: %v", err)
	default:
		if _, mergeErr := h.store.Merge(league, sales, time.Now()); mergeErr != nil {
			status, errText = HistoryError, mergeErr.Error()
			log.Printf("trade history: save: %v", mergeErr)
		}
	}
	h.mu.Lock()
	h.fetching, h.status, h.err = false, status, errText
	h.mu.Unlock()
	s.emitTradeHistory(s.GetTradeHistory())
}

func (s *AppService) emitTradeHistory(view TradeHistoryView) {
	if s.app != nil {
		s.app.Event.Emit("trade-history", view)
	}
}

// tradeHistoryLoop asks the site after start, when kicked, and when
// nothing was asked for tradeHistoryFallback.
func (s *AppService) tradeHistoryLoop() {
	first := time.NewTimer(tradeHistoryFirst)
	check := time.NewTicker(tradeHistoryCheck)
	defer check.Stop()
	for {
		select {
		case <-first.C:
		case <-s.history.kick:
		case <-check.C:
			s.history.mu.Lock()
			idle := time.Since(s.history.lastTry) >= tradeHistoryFallback
			s.history.mu.Unlock()
			if !idle {
				continue
			}
		}
		s.fetchTradeHistory()
	}
}

func (s *AppService) kickTradeHistory() {
	select {
	case s.history.kick <- struct{}{}:
	default:
	}
}

// merchantSpoke follows a line of Ange in the game's log: a sale (or a
// listing change) the history may now show. One request covers a run of
// such lines; a line inside the gap waits for its end.
func (s *AppService) merchantSpoke() {
	if on, _, _ := s.tradeHistoryOn(); !on {
		return
	}
	h := &s.history
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ange != nil {
		return
	}
	h.ange = time.AfterFunc(angeDelay(time.Since(h.lastAnge)), s.angeRequest)
}

// angeDelay is how long a merchant line waits for its request, sinceLast
// after the last such request.
func angeDelay(sinceLast time.Duration) time.Duration {
	return max(tradeHistoryAfterAnge, tradeHistoryAngeGap-sinceLast)
}

// angeRequest sends the request a merchant line waited for, or puts it off
// until the quota has room.
func (s *AppService) angeRequest() {
	h := &s.history
	if wait := s.overlayClient.HistoryNextIn(); wait > 0 {
		h.mu.Lock()
		h.ange = time.AfterFunc(wait+5*time.Second, s.angeRequest)
		h.mu.Unlock()
		return
	}
	h.mu.Lock()
	h.ange, h.lastAnge = nil, time.Now()
	h.mu.Unlock()
	s.kickTradeHistory()
}

// applyTradeHistory follows a change of the tracker settings.
func (s *AppService) applyTradeHistory(old, next overlay.Settings) {
	if next.TradeHistory && !old.TradeHistory {
		s.kickTradeHistory()
	}
	s.emitTradeHistory(s.GetTradeHistory())
	s.syncEarningsOverlay()
}

// ShowTracker opens the tracker window.
func (s *AppService) ShowTracker() {
	if s.trackerWindow == nil {
		return
	}
	s.HidePanel()
	s.trackerWindow.Show()
	s.trackerWindow.UnMinimise()
	s.trackerWindow.Focus()
}

// toggleTracker is the tracker shortcut: it opens the window, or closes it
// when open.
func (s *AppService) toggleTracker() {
	if s.trackerWindow == nil {
		return
	}
	if s.trackerWindow.IsVisible() && !s.trackerWindow.IsMinimised() {
		s.trackerWindow.Hide()
		return
	}
	s.ShowTracker()
}

// The earnings overlay's size in window pixels.
const (
	earningsWidth  = 172
	earningsHeight = 46
)

// Placing the overlay: it stops letting clicks through and stays up over the
// game while the player drags it; saving keeps its place as a share of the
// game's client area.
const (
	PlaceOK     = ""
	PlaceNoGame = "noGame" // the game is not running
)

// PlaceEarnings starts placing the earnings overlay over the game.
func (s *AppService) PlaceEarnings() string {
	w := s.earningsWindow
	if w == nil {
		return PlaceNoGame
	}
	client, ok := overlay.GameClient()
	if !ok {
		return PlaceNoGame
	}
	s.earningsPlacing.Store(true)
	w.SetIgnoreMouseEvents(false)
	w.Show()
	overlay.ShowOver(uintptr(w.NativeWindow()), s.earningsRectNow(client))
	s.emitTradeHistory(s.GetTradeHistory())
	return PlaceOK
}

// FinishEarningsPlace ends placing; with save, the overlay keeps the place
// it was dragged to.
func (s *AppService) FinishEarningsPlace(save bool) {
	w := s.earningsWindow
	if w == nil || !s.earningsPlacing.Load() {
		return
	}
	if save {
		r, okWin := overlay.WindowRect(uintptr(w.NativeWindow()))
		client, okGame := overlay.GameClient()
		if okWin && okGame && client.Dx() > 0 && client.Dy() > 0 {
			x := float64(r.Min.X-client.Min.X) / float64(client.Dx())
			y := float64(r.Min.Y-client.Min.Y) / float64(client.Dy())
			next := s.GetOverlaySettings()
			next.EarningsCorner, next.EarningsX, next.EarningsY = overlay.EarningsCustom, x, y
			if _, err := s.SaveOverlaySettings(next); err != nil {
				log.Printf("earnings overlay: save place: %v", err)
			}
		}
	}
	w.SetIgnoreMouseEvents(true)
	s.earningsPlacing.Store(false)
	s.emitTradeHistory(s.GetTradeHistory())
	s.syncEarningsOverlay()
}

// earningsRectNow is the overlay's place in client for the current settings.
func (s *AppService) earningsRectNow(client image.Rectangle) image.Rectangle {
	s.overlayMu.RLock()
	st := s.overlaySettings
	s.overlayMu.RUnlock()
	dpi := 1.0
	if screen := s.anchorScreen(); screen != nil && screen.ScaleFactor > 0 {
		dpi = float64(screen.ScaleFactor)
	}
	return earningsRect(client, st.EarningsCorner, st.EarningsX, st.EarningsY, dpi)
}

// syncEarningsOverlay shows the earnings over the game while it is the
// active window and the overlay is switched on, and hides it otherwise.
func (s *AppService) syncEarningsOverlay() {
	w := s.earningsWindow
	if w == nil || s.earningsPlacing.Load() {
		return
	}
	_, on, _ := s.tradeHistoryOn()
	if !on || !s.gameActive.Load() {
		if w.IsVisible() {
			w.Hide()
		}
		return
	}
	client, ok := overlay.GameClient()
	if !ok {
		return
	}
	w.Show()
	overlay.ShowOver(uintptr(w.NativeWindow()), s.earningsRectNow(client))
	// The window is made not to take focus; should Windows give it anyway,
	// the game gets it back.
	if !overlay.IsGameWindow(overlay.ForegroundWindow()) {
		overlay.FocusGame()
	}
}

// earningsRect places the overlay in a corner of the game's client area
// (physical pixels), clear of its edge, or where it was dragged (fx, fy:
// its top-left corner as a share of the area), kept inside the area.
func earningsRect(client image.Rectangle, corner string, fx, fy, dpi float64) image.Rectangle {
	return boxRect(client, corner, fx, fy, dpi, earningsWidth, earningsHeight)
}

// boxRect places a box of width × height window pixels like earningsRect.
func boxRect(client image.Rectangle, corner string, fx, fy, dpi float64, width, height int) image.Rectangle {
	w, h := int(float64(width)*dpi), int(float64(height)*dpi)
	margin := int(10 * dpi)
	x, y := client.Min.X+margin, client.Min.Y+margin
	switch corner {
	case overlay.EarningsCustom:
		x = client.Min.X + int(fx*float64(client.Dx()))
		y = client.Min.Y + int(fy*float64(client.Dy()))
		x = min(max(x, client.Min.X), client.Max.X-w)
		y = min(max(y, client.Min.Y), client.Max.Y-h)
	case "top-center":
		x = client.Min.X + (client.Dx()-w)/2
	case "top-right":
		x = client.Max.X - w - margin
	case "bottom-left":
		y = client.Max.Y - h - margin
	case "bottom-right":
		x, y = client.Max.X-w-margin, client.Max.Y-h-margin
	}
	return image.Rect(x, y, x+w, y+h)
}
