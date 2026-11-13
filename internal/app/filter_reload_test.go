package app

import (
	"testing"
	"time"
)

// A filter written before the game started was read as it began; one
// written later still waits for a reload.
func TestReloadStillDue(t *testing.T) {
	written := time.Date(2026, 11, 13, 23, 0, 0, 0, time.Local)
	for _, c := range []struct {
		start time.Time
		want  bool
	}{
		{time.Time{}, true},
		{written.Add(-time.Hour), true},
		{written, true},
		{written.Add(time.Minute), false},
	} {
		if got := reloadStillDue(written.UnixNano(), c.start); got != c.want {
			t.Errorf("game started %v: due %v, want %v", c.start, got, c.want)
		}
	}
}
