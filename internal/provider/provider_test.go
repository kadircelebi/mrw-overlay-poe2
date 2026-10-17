package provider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

var now = time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC)

func saveSnap(t *testing.T, path, league string, age time.Duration) {
	t.Helper()
	s := &prices.Snapshot{SchemaVersion: prices.SchemaVersion, League: league, GeneratedAt: now.Add(-age),
		Rates: prices.Rates{DivineEx: 700}, Currency: []prices.CurrencyPrice{{Name: "Exalted Orb", ValueEx: 1}}}
	if err := prices.Save(path, s); err != nil {
		t.Fatal(err)
	}
}

func TestCacheAgeAndLeague(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	saveSnap(t, path, "Rites", 5*time.Hour)
	at := func() time.Time { return now }
	if _, err := (Cache{Path: path, MaxAge: 6 * time.Hour, League: "Rites", Now: at}).Get(context.Background()); err != nil {
		t.Fatalf("recent cache refused: %v", err)
	}
	if _, err := (Cache{Path: path, MaxAge: 4 * time.Hour, League: "Rites", Now: at}).Get(context.Background()); err == nil {
		t.Fatal("old cache served")
	}
	if _, err := (Cache{Path: path, MaxAge: 6 * time.Hour, League: "Standard", Now: at}).Get(context.Background()); err == nil {
		t.Fatal("other league served")
	}
	// Without limits it serves whatever is on disk.
	if _, err := (Cache{Path: path}).Get(context.Background()); err != nil {
		t.Fatalf("plain cache: %v", err)
	}
}

type fake struct {
	name string
	err  error
}

func (f fake) Name() string { return f.name }
func (f fake) Get(context.Context) (*prices.Snapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &prices.Snapshot{}, nil
}

// Shared prices failed: the last prices serve while recent, then the app
// collects its own, and when that fails too the last prices of any age.
func TestChainFallsBackByAge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	at := func() time.Time { return now }
	chain := func(local error) Chain {
		return Chain{
			fake{"shared", errors.New("download failed")},
			Cache{Path: path, MaxAge: 6 * time.Hour, League: "Rites", Now: at},
			fake{"local", local},
			Cache{Path: path},
		}
	}
	saveSnap(t, path, "Rites", 2*time.Hour)
	if _, src, _ := chain(nil).Get(context.Background()); src != "önbellek" {
		t.Errorf("recent: %s", src)
	}
	saveSnap(t, path, "Rites", 7*time.Hour)
	if _, src, _ := chain(nil).Get(context.Background()); src != "local" {
		t.Errorf("old: %s", src)
	}
	if _, src, err := chain(errors.New("offline")).Get(context.Background()); err != nil || src != "önbellek" {
		t.Errorf("old, offline: %s %v", src, err)
	}
}
