package engine

import (
	"os"
	"testing"
	"time"

	"poe2filter/internal/filter"
)

func TestRetryDelayBacksOff(t *testing.T) {
	for _, tc := range []struct {
		fails int
		want  time.Duration
	}{
		{1, 5 * time.Minute},
		{2, 10 * time.Minute},
		{3, 20 * time.Minute},
		{4, 30 * time.Minute},
		{9, 30 * time.Minute},
	} {
		if got := retryDelay(tc.fails); got != tc.want {
			t.Errorf("retryDelay(%d) = %v, want %v", tc.fails, got, tc.want)
		}
	}
}

func TestLeaguesNeverTouchTheChoice(t *testing.T) {
	e := &Engine{}
	e.cfg = filter.Config{LeagueName: "Forbidden Rites"}

	// No list fetched yet: the built-in one is offered.
	if got := e.Leagues(); len(got) == 0 || got[0] != filter.DefaultLeagues[0] {
		t.Fatalf("varsayılan liste bekleniyordu, alınan: %v", got)
	}

	// A live list is passed through in the upstream order...
	e.leagues = []string{"Rise of the Abyssal", "Standard"}
	got := e.Leagues()
	if len(got) != 2 || got[0] != "Rise of the Abyssal" {
		t.Fatalf("canlı liste olduğu gibi dönmeli: %v", got)
	}
	// ...and it never rewrites the configured league, listed or not.
	if e.Config().LeagueName != "Forbidden Rites" {
		t.Fatalf("lig seçimi değişmiş: %q", e.Config().LeagueName)
	}

	// The returned slice is a copy: a caller cannot corrupt the cache.
	got[0] = "değiştirildi"
	if e.Leagues()[0] != "Rise of the Abyssal" {
		t.Fatal("Leagues() iç listeyi paylaşıyor")
	}
}

// The panel's "settings changed, update" notice comes from ConfigPending: it
// must clear once the filter matches the settings again, whether by writing
// the filter or by undoing the change, and must not fire for app-only settings.
func TestConfigPendingFollowsTheWrittenFilter(t *testing.T) {
	dir := t.TempDir()
	e := New(Options{Dir: dir})
	if e.State().ConfigPending {
		t.Fatal("pending before any filter was written")
	}
	written := e.Config()
	// What a successful run records.
	if err := os.MkdirAll(e.dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.filterKeyPath(), []byte(written.FilterKey()), 0o644); err != nil {
		t.Fatal(err)
	}
	e = New(Options{Dir: dir}) // the key survives a restart
	if e.State().ConfigPending {
		t.Fatal("pending right after the filter was written")
	}

	changed := written
	changed.MinValue = written.MinValue + 10
	if _, err := e.SetConfig(changed); err != nil {
		t.Fatal(err)
	}
	if !e.State().ConfigPending {
		t.Fatal("a threshold change is not pending")
	}
	if _, err := e.SetConfig(written); err != nil {
		t.Fatal(err)
	}
	if e.State().ConfigPending {
		t.Fatal("still pending after the change was undone")
	}

	appOnly := written
	appOnly.Language = "tr"
	appOnly.AutoUpdateHours = written.AutoUpdateHours + 2
	if _, err := e.SetConfig(appOnly); err != nil {
		t.Fatal(err)
	}
	if e.State().ConfigPending {
		t.Fatal("an app-only setting asks for a filter update")
	}
}
