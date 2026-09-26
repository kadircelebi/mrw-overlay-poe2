package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"poe2filter/internal/gamesounds"
	"poe2filter/internal/i18n"
	"poe2filter/internal/overlay"
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
			return s.live.States(), nil
		}
	}
	return s.live.States(), errors.New(i18n.T("live.err.missing"))
}

func (s *AppService) StopLiveSearch(id string) []trade.LiveState {
	s.live.Stop(id)
	return s.live.States()
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
