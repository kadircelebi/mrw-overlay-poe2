package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"poe2filter/internal/gamesounds"
	"poe2filter/internal/i18n"
	"poe2filter/internal/overlay"
	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
)

// Live search: saved market searches the player starts from the market's
// Live Search tab. New listings raise a Windows notification and the chosen
// game sound; the market lists them with the usual hideout button.

// liveAlerts keeps the sound from piling up when several searches fire at
// once.
type liveAlerts struct {
	mu        sync.Mutex
	lastSound time.Time
}

func newLiveManager(s *AppService) *trade.LiveManager {
	return trade.NewLiveManager(trade.LiveOptions{
		Client:   s.overlayClient,
		OnChange: s.liveChanged,
		OnFound:  s.liveFound,
		Describe: describeLiveError,
		Log:      s.liveLog,
	})
}

// liveLog appends to <data>\live.log what the live sockets do (connects,
// GGG's frames, fetch errors), so a search that finds nothing can be
// diagnosed. It never holds the session. The file restarts past 512 KB.
func (s *AppService) liveLog(msg string) {
	log.Print(msg)
	path := filepath.Join(s.meta.DataDir, "live.log")
	s.liveAlert.mu.Lock()
	defer s.liveAlert.mu.Unlock()
	flag := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if fi, err := os.Stat(path); err == nil && fi.Size() > 512<<10 {
		flag = os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\r\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}

func describeLiveError(err error) string {
	var apiErr *trade.APIError
	switch {
	case errors.Is(err, trade.ErrNotSignedIn):
		return i18n.T("live.err.login")
	case errors.Is(err, trade.ErrLiveLimit):
		return i18n.T("live.err.limit", trade.MaxLiveSearches)
	case errors.Is(err, trade.ErrLiveAuth):
		return i18n.T("live.err.auth")
	case errors.As(err, &apiErr) && apiErr.Status == 429:
		return i18n.T("live.err.quota")
	case errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403):
		return i18n.T("live.err.auth")
	case errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500:
		return i18n.T("live.err.query")
	}
	return i18n.T("live.err.network", err)
}

func (s *AppService) liveChanged() {
	if s.app != nil {
		s.app.Event.Emit("live-state", s.live.States())
	}
}

func (s *AppService) liveFound(state trade.LiveState, listings []trade.EvaluatedListing) {
	s.tagListingStats(listings)
	if s.app != nil {
		s.app.Event.Emit("live-found", state)
	}
	s.overlayMu.RLock()
	settings := s.overlaySettings
	s.overlayMu.RUnlock()

	if settings.LiveNotify && s.notify != nil {
		first := listings[0]
		name := first.Item.Name
		if name == "" {
			name = first.Item.BaseType
		}
		price := s.formatPrice(first.Amount, first.Currency)
		body := i18n.T("live.notifyOne", name, price)
		if len(listings) > 1 {
			body = i18n.T("live.notifyMany", len(listings), name, price)
		}
		s.notify("live-"+state.ID, i18n.T("live.notifyTitle", state.Name), body)
	}
	if settings.LiveSound != "none" && gamesounds.Known(settings.LiveSound) {
		s.liveAlert.mu.Lock()
		due := time.Since(s.liveAlert.lastSound) > 2*time.Second
		if due {
			s.liveAlert.lastSound = time.Now()
		}
		s.liveAlert.mu.Unlock()
		if due {
			if path, err := gamesounds.Ensure(s.meta.DataDir, settings.LiveSound); err == nil {
				_ = playSound(path)
			}
		}
	}
}

// formatPrice names the currency the way the trade site does ("5 × Divine
// Orb"), falling back to GGG's id when the catalog is not loaded.
func (s *AppService) formatPrice(amount float64, currency string) string {
	name := currency
	if catalog, err := s.overlayCatalog.Load(context.Background()); err == nil {
		for _, c := range catalog.Currencies {
			if c.ID == currency {
				name = c.Text
				break
			}
		}
	}
	return fmt.Sprintf("%g × %s", amount, name)
}

// LiveSearches lists the live searches started in this session.
func (s *AppService) LiveSearches() []trade.LiveState {
	return s.live.States()
}

// StartLiveSearch opens a live search for a saved search.
func (s *AppService) StartLiveSearch(id string) ([]trade.LiveState, error) {
	library, err := s.overlaySearches.List()
	if err != nil {
		return s.live.States(), err
	}
	for _, saved := range library.Searches {
		if saved.ID == id {
			if err := s.live.Start(saved.ID, saved.Name, saved.Query); err != nil {
				return s.live.States(), errors.New(describeLiveError(err))
			}
			s.rememberLive(id, true)
			return s.live.States(), nil
		}
	}
	return s.live.States(), errors.New(i18n.T("live.err.missing"))
}

func (s *AppService) StopLiveSearch(id string) []trade.LiveState {
	s.live.Stop(id)
	s.rememberLive(id, false)
	return s.live.States()
}

// ---- Remembered live searches -----------------------------------------------
// The searches the player started (and did not stop) are kept in
// live_searches.json and started again when the app starts; quitting the app
// stops them without forgetting them. Each restart costs one search.

type liveFile struct {
	IDs []string `json:"ids"`
}

func (s *AppService) livePath() string { return filepath.Join(s.meta.DataDir, "live_searches.json") }

func (s *AppService) rememberedLive() []string {
	var f liveFile
	if raw, err := os.ReadFile(s.livePath()); err == nil {
		_ = json.Unmarshal(raw, &f)
	}
	return f.IDs
}

func (s *AppService) rememberLive(id string, on bool) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	ids := slices.DeleteFunc(s.rememberedLive(), func(x string) bool { return x == id })
	if on {
		ids = append(ids, id)
	}
	raw, err := json.Marshal(liveFile{IDs: ids})
	if err == nil {
		_ = prices.WriteFileAtomic(s.livePath(), raw)
	}
}

// restoreLiveSearches starts the remembered searches again. Without a
// pathofexile.com session it does nothing and keeps the list for later.
func (s *AppService) restoreLiveSearches(ctx context.Context) {
	ids := s.rememberedLive()
	if len(ids) == 0 {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second): // let the session and catalog load
	}
	if !s.overlayClient.SignedIn() {
		s.liveLog("restore: not signed in, remembered searches wait")
		return
	}
	library, err := s.overlaySearches.List()
	if err != nil {
		return
	}
	for _, id := range ids {
		found := false
		for _, saved := range library.Searches {
			if saved.ID == id {
				found = true
				if err := s.live.Start(saved.ID, saved.Name, saved.Query); err != nil {
					s.liveLog(fmt.Sprintf("restore %s: %v", saved.Name, err))
				}
			}
		}
		if !found {
			s.rememberLive(id, false)
		}
	}
}

// LiveResults are a live search's found listings, newest first.
func (s *AppService) LiveResults(id string) []trade.EvaluatedListing {
	return s.live.Results(id)
}

func (s *AppService) ClearLiveResults(id string) []trade.LiveState {
	s.live.Clear(id)
	return s.live.States()
}

// SetLiveAlerts changes how a live search announces a find, without touching
// the overlay's shortcuts.
func (s *AppService) SetLiveAlerts(sound string, notify bool) (overlay.Settings, error) {
	if sound != "none" && !gamesounds.Known(sound) {
		return s.GetOverlaySettings(), errors.New(i18n.T("err.soundInvalid"))
	}
	s.overlayMu.Lock()
	next := s.overlaySettings
	next.LiveSound, next.LiveNotify = sound, notify
	err := overlay.SaveSettings(s.overlaySettingsPath, next)
	if err == nil {
		s.overlaySettings = next
	}
	s.overlayMu.Unlock()
	return next, err
}
