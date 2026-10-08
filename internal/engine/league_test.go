package engine

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"poe2filter/internal/filter"
)

// serveLeagues makes refreshLeagues see list, and lets it fetch right away.
func serveLeagues(t *testing.T, e *Engine, list ...string) {
	t.Helper()
	prev := fetchLeagues
	fetchLeagues = func(context.Context, *http.Client) ([]string, error) { return list, nil }
	t.Cleanup(func() { fetchLeagues = prev })
	e.leagueMu.Lock()
	e.leaguesAt, e.leaguesTried = time.Time{}, time.Time{}
	e.leagueMu.Unlock()
}

func TestAutomaticLeagueFollowsANewLeague(t *testing.T) {
	e := newTestEngine(t)
	var notes []string
	e.opt.Notify = func(title, _ string) { notes = append(notes, title) }
	if cfg := e.Config(); !cfg.LeagueAuto {
		t.Fatal("a fresh install should follow the current league")
	}

	serveLeagues(t, e, "Forbidden Rites", "HC Forbidden Rites", "Runes of Aldur", "Standard")
	if e.refreshLeagues(context.Background()) || e.Config().LeagueName != "Forbidden Rites" {
		t.Fatalf("same league should not switch: %q", e.Config().LeagueName)
	}

	serveLeagues(t, e, "Next League", "HC Next League", "Forbidden Rites", "Runes of Aldur", "Standard")
	if !e.refreshLeagues(context.Background()) {
		t.Fatal("a new league should switch")
	}
	if got := e.Config().LeagueName; got != "Next League" {
		t.Fatalf("league = %q", got)
	}
	// A window still holding the old league must not move it back.
	stale := e.Config()
	stale.LeagueName = "Forbidden Rites"
	if saved, _ := e.SetConfig(stale); saved.LeagueName != "Next League" {
		t.Fatalf("stale save moved the league to %q", saved.LeagueName)
	}
	if len(notes) != 0 {
		t.Fatalf("the switch itself should not notify (the filter update does): %v", notes)
	}
}

func TestHandPickedLeagueStaysAndIsAnnouncedOnce(t *testing.T) {
	e := newTestEngine(t)
	var notes []string
	e.opt.Notify = func(title, _ string) { notes = append(notes, title) }
	cfg := e.Config()
	cfg.LeagueName, cfg.LeagueAuto = "HC Forbidden Rites", false
	if _, err := e.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	serveLeagues(t, e, "Next League", "HC Next League", "Standard")
	if e.refreshLeagues(context.Background()) {
		t.Fatal("a hand-picked league must not switch")
	}
	if got := e.Config().LeagueName; got != "HC Forbidden Rites" {
		t.Fatalf("league = %q", got)
	}
	serveLeagues(t, e, "Next League", "HC Next League", "Standard")
	e.refreshLeagues(context.Background())
	if len(notes) != 1 {
		t.Fatalf("an ended league should be announced once, got %v", notes)
	}

	// Choosing Automatic again takes the current league at once.
	cfg = e.Config()
	cfg.LeagueAuto = true
	if saved, _ := e.SetConfig(cfg); saved.LeagueName != "Next League" {
		t.Fatalf("automatic = %q", saved.LeagueName)
	}
}

func TestOldConfigOnTheDefaultLeagueBecomesAutomatic(t *testing.T) {
	for _, tc := range []struct {
		league string
		auto   bool
	}{{"Forbidden Rites", true}, {"HC Forbidden Rites", false}, {"Standard", false}} {
		dir := t.TempDir()
		data := []byte(`{"league_name":"` + tc.league + `","min_value":5}`)
		if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		e := New(Options{Dir: dir})
		if got := e.Config(); got.LeagueAuto != tc.auto || got.LeagueName != tc.league {
			t.Errorf("%s: auto=%v league=%q", tc.league, got.LeagueAuto, got.LeagueName)
		}
		// Saved and read back, the choice stays.
		if got := filter.LoadConfig(filepath.Join(dir, "config.json")); got.LeagueAuto != tc.auto {
			t.Errorf("%s: after save auto=%v", tc.league, got.LeagueAuto)
		}
	}
}
