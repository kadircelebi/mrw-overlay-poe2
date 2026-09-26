package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/i18n"
	"poe2filter/internal/overlay"
	"poe2filter/internal/trade"
)

const overlayEvaluationCacheTTL = 10 * time.Minute

type overlayEvaluationCacheEntry struct {
	result    trade.Evaluation
	expiresAt time.Time
}

type overlayEvaluationFlight struct {
	done   chan struct{}
	result trade.Evaluation
	err    error
}

func (s *AppService) GetOverlaySettings() overlay.Settings {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlaySettings
}

func (s *AppService) SaveOverlaySettings(next overlay.Settings) (overlay.Settings, error) {
	next.Normalize()
	s.overlayMu.RLock()
	old := s.overlaySettings
	s.overlayMu.RUnlock()
	if s.rebindOverlay != nil {
		if err := s.rebindOverlay(old, next); err != nil {
			return old, err
		}
	}
	if err := overlay.SaveSettings(s.overlaySettingsPath, next); err != nil {
		if s.rebindOverlay != nil {
			_ = s.rebindOverlay(next, old)
		}
		return old, err
	}
	s.overlayMu.Lock()
	s.overlaySettings = next
	s.overlayMu.Unlock()
	s.applyOverlayScale()
	return next, nil
}

func (s *AppService) GetTradeCatalog() (overlay.Catalog, error) {
	return s.overlayCatalog.Load(context.Background())
}

// TradeCurrencies lists the currencies listings are priced in, with icons,
// without sending the whole stat catalog to a window that only shows prices.
func (s *AppService) TradeCurrencies() ([]overlay.CurrencyEntry, error) {
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		return nil, err
	}
	return catalog.Currencies, nil
}

// QuoteCurrency prices a stackable item from the filter's price snapshot.
func (s *AppService) QuoteCurrency(name string) overlay.CurrencyQuote {
	return overlay.QuoteCurrency(s.eng.Prices(), name)
}

// UniqueIcons maps unique names to their art, from the last price snapshot
// (poe.ninja sends an icon with each unique). An unidentified unique shows
// these so the player can tell the candidates apart by their look.
func (s *AppService) UniqueIcons() map[string]string {
	out := map[string]string{}
	snap := s.eng.Prices()
	if snap == nil {
		return out
	}
	for _, base := range snap.UniqueBases {
		for _, u := range base.Uniques {
			if u.Icon != "" && out[u.Name] == "" {
				out[u.Name] = u.Icon
			}
		}
	}
	return out
}

// StatTiers returns the modifier tier tables of a base, or of an item class
// when the search names only a category. An empty list means none are known
// (the tiers could not be built yet, or the item rolls no tiered stats).
func (s *AppService) StatTiers(baseType, itemClass string) ([]overlay.TierTable, error) {
	data, err := s.overlayTiers.Load(context.Background())
	if err != nil {
		return nil, err
	}
	return data.For(baseType, itemClass), nil
}

func (s *AppService) RefreshTradeCatalog() (overlay.Catalog, error) {
	return s.overlayCatalog.Refresh(context.Background())
}

func (s *AppService) GetOverlaySnapshot() overlay.Snapshot {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlaySnapshot
}

func (s *AppService) GetOverlayDraft() trade.EvaluateRequest {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlayDraft
}

func (s *AppService) GetSavedOverlaySearches() (overlay.SearchLibrary, error) {
	return s.overlaySearches.List()
}

func (s *AppService) SaveOverlaySearch(name, folder string, query trade.EvaluateRequest) (overlay.SearchLibrary, error) {
	return s.overlaySearches.Save(name, folder, query)
}

func (s *AppService) DeleteOverlaySearch(id string) (overlay.SearchLibrary, error) {
	s.live.Forget(id)
	return s.overlaySearches.Delete(id)
}

// MoveOverlaySearch puts a saved search into a folder ("" = top level).
func (s *AppService) MoveOverlaySearch(id, folder string) (overlay.SearchLibrary, error) {
	return s.overlaySearches.Move(id, folder)
}

func (s *AppService) CreateSearchFolder(name string) (overlay.SearchLibrary, error) {
	return s.overlaySearches.CreateFolder(name)
}

func (s *AppService) RenameSearchFolder(id, name string) (overlay.SearchLibrary, error) {
	return s.overlaySearches.RenameFolder(id, name)
}

// DeleteSearchFolder removes a folder and moves its searches to the top level.
func (s *AppService) DeleteSearchFolder(id string) (overlay.SearchLibrary, error) {
	return s.overlaySearches.DeleteFolder(id)
}

// ParseOverlayText is also useful for diagnostics: a copied item can be pasted
// into the expanded market without needing to synthesize game input.
func (s *AppService) ParseOverlayText(raw string) (overlay.Snapshot, error) {
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		return overlay.Snapshot{}, err
	}
	item, err := overlay.ParseItem(raw, catalog)
	if err != nil {
		return overlay.Snapshot{}, err
	}
	snap := overlay.Snapshot{Item: &item}
	s.setOverlaySnapshot(snap)
	return snap, nil
}

// EvaluateOverlay reuses identical successful searches for ten minutes unless
// refresh is true. In-flight requests are shared as well, so opening Market
// while the compact overlay is evaluating cannot consume a second GGG search.
func (s *AppService) EvaluateOverlay(in trade.EvaluateRequest, refresh bool) (trade.Evaluation, error) {
	if in.League == "" {
		in.League = s.eng.Config().LeagueName
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		return trade.Evaluation{}, err
	}
	key := string(encoded)
	now := time.Now()

	s.overlayEvalMu.Lock()
	for cachedKey, cached := range s.overlayEvalCache {
		if !cached.expiresAt.After(now) {
			delete(s.overlayEvalCache, cachedKey)
		}
	}
	if !refresh {
		if cached, ok := s.overlayEvalCache[key]; ok {
			s.overlayEvalMu.Unlock()
			return cached.result, nil
		}
	}
	if flight, ok := s.overlayEvalFlights[key]; ok {
		s.overlayEvalMu.Unlock()
		<-flight.done
		return flight.result, flight.err
	}
	flight := &overlayEvaluationFlight{done: make(chan struct{})}
	s.overlayEvalFlights[key] = flight
	s.overlayEvalMu.Unlock()

	result, evalErr := s.evaluateOverlayFresh(in)
	s.overlayEvalMu.Lock()
	flight.result, flight.err = result, evalErr
	if evalErr == nil {
		s.overlayEvalCache[key] = overlayEvaluationCacheEntry{result: result, expiresAt: time.Now().Add(overlayEvaluationCacheTTL)}
	}
	delete(s.overlayEvalFlights, key)
	close(flight.done)
	s.overlayEvalMu.Unlock()
	return result, evalErr
}

func (s *AppService) evaluateOverlayFresh(in trade.EvaluateRequest) (trade.Evaluation, error) {
	if wait := s.overlayClient.Search.NextIn(); wait > 5*time.Second {
		return trade.Evaluation{}, errors.New(quotaWaitMessage(s.overlayClient.Search.Status()))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := s.overlayClient.Evaluate(ctx, in)
	if err == nil {
		s.tagListingStats(result.Listings)
		result.SignedIn = s.overlayClient.SignedIn()
		return result, nil
	}
	var apiErr *trade.APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.Status == 429:
		return trade.Evaluation{}, errors.New(quotaPenaltyMessage(s.overlayClient.Search.Status()))
	case strings.Contains(strings.ToLower(err.Error()), "content exceeded"):
		return trade.Evaluation{}, errors.New(i18n.T("overlay.err.tooBroad"))
	case strings.Contains(strings.ToLower(err.Error()), "query is too complex"):
		// Measured: without a login GGG refuses any Weighted Sum group, even
		// one with a single stat, while And and Count groups pass.
		if hasWeightGroup(in) && !s.overlayClient.SignedIn() {
			return trade.Evaluation{}, errors.New(i18n.T("overlay.err.weightNeedsLogin"))
		}
		if s.overlayClient.SignedIn() {
			// GGG adds this hint only for requests it did not count as signed in.
			if strings.Contains(strings.ToLower(err.Error()), "logging in will increase") {
				return trade.Evaluation{}, errors.New(i18n.T("overlay.err.sessionIgnored"))
			}
			return trade.Evaluation{}, errors.New(i18n.T("overlay.err.tooComplex"))
		}
		return trade.Evaluation{}, errors.New(i18n.T("overlay.err.tooComplexSignedOut"))
	case errors.Is(err, context.DeadlineExceeded):
		return trade.Evaluation{}, errors.New(i18n.T("overlay.err.searchTimeout"))
	default:
		return trade.Evaluation{}, err
	}
}

// FetchOverlayListings loads the next page of a search the overlay already
// ran. It uses fetch quota only, so the result list can grow as it scrolls.
func (s *AppService) FetchOverlayListings(searchID string, ids []string) ([]trade.EvaluatedListing, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listings, err := s.overlayClient.FetchEvaluated(ctx, searchID, ids)
	var apiErr *trade.APIError
	switch {
	case err == nil:
		s.tagListingStats(listings)
		return listings, nil
	case errors.As(err, &apiErr) && apiErr.Status == 429:
		return nil, errors.New(i18n.T("overlay.err.fetchLimit"))
	case errors.Is(err, context.DeadlineExceeded):
		return nil, errors.New(i18n.T("overlay.err.fetchTimeout"))
	default:
		return nil, err
	}
}

// tagListingStats gives every listing mod line its trade stat, so Market can
// sort by any affix it shows. GGG names the stat of each line itself; the
// catalog wording is the fallback for a line that came without one.
func (s *AppService) tagListingStats(listings []trade.EvaluatedListing) {
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		return
	}
	for li := range listings {
		item := &listings[li].Item
		for mi := range item.Mods {
			mod := &item.Mods[mi]
			if mod.StatID != "" {
				continue
			}
			kind := mod.Type
			if kind == "fractured" || kind == "desecrated" {
				kind = "explicit"
			}
			mod.StatID = overlay.ListingStatID(mod.Description, kind, item.StatHashes, catalog)
		}
	}
}

// TravelToHideout takes the player to the seller's hideout for an instant
// buyout listing (needs the pathofexile.com session and the game running on
// the same account).
func (s *AppService) TravelToHideout(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := s.overlayClient.TravelToHideout(ctx, token)
	var apiErr *trade.APIError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, trade.ErrNotSignedIn):
		return errors.New(i18n.T("overlay.err.hideoutNeedsLogin"))
	case errors.Is(err, trade.ErrHideoutDenied):
		return errors.New(i18n.T("overlay.err.hideoutDenied"))
	case errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403):
		return errors.New(i18n.T("overlay.err.hideoutSession"))
	case errors.As(err, &apiErr) && apiErr.Status == 429:
		return errors.New(i18n.T("overlay.err.hideoutLimit"))
	default:
		return err
	}
}

// OverlayQuota reports how full GGG's search windows are for this IP, so the
// overlay can show who is spending the quota and why a search waits.
func (s *AppService) OverlayQuota() trade.QuotaStatus {
	return s.overlayClient.Search.Status()
}

func quotaWaitMessage(st trade.QuotaStatus) string {
	wait := formatWait(st.WaitSec)
	switch st.WaitReason {
	case trade.WaitPenalty:
		return quotaPenaltyMessage(st)
	case trade.WaitShared:
		w := quotaWindow(st, st.WaitWindowSec)
		return i18n.T("overlay.quota.shared", windowLabel(st.WaitWindowSec), w.Hits, w.Limit, wait)
	case trade.WaitOurs:
		w := quotaWindow(st, st.WaitWindowSec)
		return i18n.T("overlay.quota.ours", windowLabel(st.WaitWindowSec), w.Allowed, w.Limit, wait)
	}
	return i18n.T("overlay.quota.wait", wait)
}

func quotaPenaltyMessage(st trade.QuotaStatus) string {
	wait := formatWait(int(time.Until(st.RestrictedUntil).Seconds() + 0.999))
	if st.RestrictedWindowSec > 0 {
		w := quotaWindow(st, st.RestrictedWindowSec)
		return i18n.T("overlay.quota.penaltyWindow", windowLabel(st.RestrictedWindowSec), w.Limit, wait)
	}
	return i18n.T("overlay.quota.penalty", wait)
}

func quotaWindow(st trade.QuotaStatus, periodSec int) trade.QuotaWindow {
	for _, w := range st.Windows {
		if w.PeriodSec == periodSec {
			return w
		}
	}
	return trade.QuotaWindow{PeriodSec: periodSec}
}

func windowLabel(sec int) string {
	switch {
	case sec >= 3600 && sec%3600 == 0:
		return i18n.T("overlay.window.hours", sec/3600)
	case sec >= 60 && sec%60 == 0:
		return i18n.T("overlay.window.minutes", sec/60)
	default:
		return i18n.T("overlay.window.seconds", sec)
	}
}

func formatWait(sec int) string {
	switch {
	case sec <= 0:
		return i18n.T("overlay.wait.soon")
	case sec < 60:
		return i18n.T("overlay.wait.sec", sec)
	case sec < 3600:
		if sec%60 == 0 {
			return i18n.T("overlay.wait.min", sec/60)
		}
		return i18n.T("overlay.wait.minSec", sec/60, sec%60)
	default:
		return i18n.T("overlay.wait.hourMin", sec/3600, sec%3600/60)
	}
}

func (s *AppService) HideOverlay() {
	if s.overlayWindow != nil {
		s.overlayWindow.Hide()
	}
}

func (s *AppService) HideMarket() {
	if s.marketWindow != nil {
		s.marketWindow.Hide()
	}
}

func (s *AppService) ShowMarket() {
	if s.marketWindow == nil {
		return
	}
	s.marketWindow.EmitEvent("overlay-query", s.GetOverlayDraft())
	s.showMarketWindow()
}

func (s *AppService) ShowMarketWithQuery(in trade.EvaluateRequest) {
	s.overlayMu.Lock()
	s.overlayDraft = in
	s.overlayMu.Unlock()
	if s.marketWindow == nil {
		return
	}
	s.marketWindow.EmitEvent("overlay-query", in)
	s.showMarketWindow()
}

// PreviewOverlay opens the compact window without touching the clipboard. It
// lets users verify placement and scale from Settings; the latest item remains
// visible when one has already been captured.
func (s *AppService) PreviewOverlay() {
	snap := s.GetOverlaySnapshot()
	if snap.Item == nil && snap.Error == "" {
		snap.Error = i18n.T("overlay.hover")
	}
	s.showOverlaySnapshot(snap)
}

func (s *AppService) OpenTradePage(rawURL string) error {
	if !strings.HasPrefix(rawURL, "https://www.pathofexile.com/trade2/search/poe2/") {
		return errors.New("invalid Path of Exile trade URL")
	}
	return s.app.Browser.OpenURL(rawURL)
}

func (s *AppService) captureOverlay() {
	s.overlayMu.RLock()
	enabled := s.overlaySettings.Enabled
	s.overlayMu.RUnlock()
	if !enabled {
		return
	}
	const sentinel = "__MRW_POE2_OVERLAY_CAPTURE__"
	s.app.Clipboard.SetText(sentinel)
	// The shortcut is global, so it also fires while our own window has focus;
	// the copy keys go to the focused window and must reach the game instead.
	switched, _ := overlay.FocusGame()
	if err := overlay.CopyAdvancedItem(switched); err != nil {
		s.showOverlaySnapshot(overlay.Snapshot{Error: err.Error()})
		return
	}
	var raw string
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(35 * time.Millisecond)
		if text, ok := s.app.Clipboard.Text(); ok && text != "" && text != sentinel {
			raw = text
			if strings.Contains(text, "Item Class:") && strings.Contains(text, "Rarity:") {
				break
			}
		}
	}
	if raw == "" || raw == sentinel {
		s.showOverlaySnapshot(overlay.Snapshot{Error: i18n.T("overlay.noCopy")})
		return
	}
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		s.showOverlaySnapshot(overlay.Snapshot{Error: i18n.T("overlay.catalogFailed", err)})
		return
	}
	item, err := overlay.ParseItem(raw, catalog)
	if err != nil {
		// Whatever the parser tripped on, the copy was not a game item.
		s.showOverlaySnapshot(overlay.Snapshot{Error: i18n.T("overlay.notAnItem")})
		return
	}
	s.showOverlaySnapshot(overlay.Snapshot{Item: &item})
}

func (s *AppService) setOverlaySnapshot(snap overlay.Snapshot) {
	s.overlayMu.Lock()
	s.overlaySnapshot = snap
	s.overlayDraft = trade.EvaluateRequest{}
	s.overlayMu.Unlock()
}

func (s *AppService) showOverlaySnapshot(snap overlay.Snapshot) {
	s.setOverlaySnapshot(snap)
	if s.overlayWindow == nil {
		return
	}
	s.confineWindows()
	s.applyOverlayScale()
	s.positionOverlayWindow(s.overlayWindow)
	s.overlayWindow.EmitEvent("overlay-item", snap)
	s.overlayWindow.Show()
	s.overlayWindow.Focus()
}

// anchorScreen is the monitor the game runs on, or the cursor's monitor when
// the game is not running.
func (s *AppService) anchorScreen() *application.Screen {
	x, y, ok := overlay.GameCenter()
	if !ok {
		x, y, ok = overlay.CursorPosition()
	}
	if ok {
		if screen := s.app.Screen.ScreenNearestPhysicalPoint(application.Point{X: x, Y: y}); screen != nil {
			return screen
		}
	}
	return s.app.Screen.GetPrimary()
}

func (s *AppService) positionOverlayWindow(window application.Window) {
	if screen := s.anchorScreen(); screen != nil {
		window.SetScreen(screen)
	}
	window.Center()
	overlay.PlaceInGame(uintptr(window.NativeWindow()), true)
}

// confineWindows keeps the overlay and market inside the game window while
// they are dragged. The window procedure must be replaced on the UI thread.
func (s *AppService) confineWindows() {
	s.confineOnce.Do(func() {
		application.InvokeSync(func() {
			for _, w := range []application.Window{s.overlayWindow, s.marketWindow} {
				if w != nil {
					overlay.Confine(uintptr(w.NativeWindow()))
				}
			}
		})
	})
}

// watchGameFocus hides the overlay windows when the user switches from the
// game to another application. Clicks inside the game are handled by the
// compact overlay's HideOnFocusLost; the market stays open for those.
func (s *AppService) watchGameFocus() {
	own := uint32(os.Getpid())
	var last uintptr
	for range time.Tick(250 * time.Millisecond) {
		fg := overlay.ForegroundWindow()
		if fg == last || fg == 0 {
			continue
		}
		last = fg
		if overlay.WindowPID(fg) == own || overlay.IsGameWindow(fg) {
			continue
		}
		for _, w := range []application.Window{s.overlayWindow, s.marketWindow} {
			if w != nil && w.IsVisible() {
				w.Hide()
			}
		}
	}
}

// showMarketWindow makes the expanded market a monitor-sized workspace rather
// than a scaled modal. UI scale still controls the contents, but must never
// make the window itself spill outside (or occupy only part of) the monitor.
// toggleMarketFromHotkey opens the market window as it was left (a second
// press hides it), without reading an item from the game.
func (s *AppService) toggleMarketFromHotkey() {
	if s.marketWindow == nil {
		return
	}
	if s.marketWindow.IsVisible() && s.marketWindow.IsFocused() {
		s.marketWindow.Hide()
		return
	}
	s.showMarketWindow()
}

func (s *AppService) showMarketWindow() {
	if s.marketWindow == nil {
		return
	}

	s.confineWindows()
	screen := s.anchorScreen()
	if screen != nil {
		s.overlayMu.RLock()
		settings := s.overlaySettings
		s.overlayMu.RUnlock()
		s.marketWindow.UnMaximise()
		s.marketWindow.SetScreen(screen)
		bounds := screen.WorkArea
		bounds.Width = max(560, bounds.Width/3)
		if bounds.Width > screen.WorkArea.Width {
			bounds.Width = screen.WorkArea.Width
		}
		s.marketWindow.SetBounds(bounds)
		s.marketWindow.SetZoom(marketScaleForScreen(settings, screen))
		overlay.PlaceInGame(uintptr(s.marketWindow.NativeWindow()), false)
	} else {
		s.marketWindow.Maximise()
	}

	s.marketWindow.Show()
	s.marketWindow.Focus()
}

func marketScaleForScreen(settings overlay.Settings, screen *application.Screen) float64 {
	scale := overlayScaleForScreen(settings, screen) * 0.82
	if scale < 0.70 {
		return 0.70
	}
	if scale > 1.45 {
		return 1.45
	}
	return scale
}

func overlayScaleForScreen(settings overlay.Settings, screen *application.Screen) float64 {
	scale := float64(settings.UIScale) / 100
	if settings.AutoScale && screen != nil {
		switch {
		case screen.PhysicalBounds.Height >= 2000:
			scale *= 1.25
		case screen.PhysicalBounds.Height >= 1400:
			scale *= 1.10
		}
	}
	if scale < 0.75 {
		return 0.75
	}
	if scale > 2.0 {
		return 2.0
	}
	return scale
}

func (s *AppService) applyOverlayScale() {
	s.overlayMu.RLock()
	settings := s.overlaySettings
	s.overlayMu.RUnlock()
	var screen *application.Screen
	if s.app != nil {
		screen = s.app.Screen.GetPrimary()
	}
	scale := overlayScaleForScreen(settings, screen)
	if s.overlayWindow != nil {
		s.overlayWindow.SetSize(int(520*scale), int(760*scale))
		s.overlayWindow.SetZoom(scale)
	}
	if s.marketWindow != nil {
		s.marketWindow.SetZoom(marketScaleForScreen(settings, screen))
	}
}

func hasWeightGroup(in trade.EvaluateRequest) bool {
	for _, group := range in.Groups {
		if kind := strings.ToLower(group.Type); kind == "weight" || kind == "weight2" {
			return true
		}
	}
	return false
}
