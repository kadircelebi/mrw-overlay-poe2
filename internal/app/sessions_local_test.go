package app

import (
	"os"
	"testing"
	"time"

	"poe2filter/internal/lootlog"
	"poe2filter/internal/overlay"
)

// TestSessionsFromTheRealLog reads the game's log at SESSIONS_LOG into a
// store in a temporary folder, twice, and prints the sessions.
func TestSessionsFromTheRealLog(t *testing.T) {
	path := os.Getenv("SESSIONS_LOG")
	if path == "" {
		t.Skip("SESSIONS_LOG not set")
	}
	store := lootlog.New(t.TempDir())
	follow := func() {
		began := time.Now()
		marks, next, again, err := overlay.ReadMarks(path, overlay.LogPos(store.LogPos()))
		if err != nil {
			t.Fatal(err)
		}
		lines := make([]lootlog.Line, len(marks))
		for i, m := range marks {
			lines[i] = lootlog.Line{Mark: lootlog.Mark{At: m.At, AFK: m.AFK}, Open: m.Open, Prev: m.Prev}
			if m.Area != nil {
				a := lootArea(*m.Area)
				lines[i].Area = &a
			}
		}
		if err := store.Follow("L", lines, lootlog.LogPos(next), again); err != nil {
			t.Fatal(err)
		}
		t.Logf("read %d marks in %v (again %v), now at %d of the log, last line %v", len(marks), time.Since(began), again, next.Offset, next.Last)
	}
	follow()
	follow()
	for i, s := range store.Sessions("L") {
		tm, sum := s.Clock(store.LogPos().Last)
		t.Logf("session %d: %v → %v, %d marks, active %v, in maps %v, home %v, breaks %v, %d maps, live %q", i+1, s.Start.Format("01-02 15:04:05"), s.End, len(s.Marks), time.Duration(sum.Active)*time.Second, time.Duration(sum.InMaps)*time.Second, time.Duration(sum.InRest)*time.Second, time.Duration(sum.Breaks)*time.Second, sum.Maps, tm.Live)
	}
}
