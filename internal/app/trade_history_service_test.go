package app

import (
	"testing"
	"time"
)

// A merchant line asks soon after the last request is a gap ago, and waits
// for the gap's end otherwise: 3 hours hold at most 12 such requests.
func TestAngeDelayKeepsTheGap(t *testing.T) {
	cases := []struct {
		since, want time.Duration
	}{
		{24 * time.Hour, tradeHistoryAfterAnge},
		{tradeHistoryAngeGap, tradeHistoryAfterAnge},
		{5 * time.Minute, 10 * time.Minute},
		{14*time.Minute + 30*time.Second, tradeHistoryAfterAnge},
		{0, tradeHistoryAngeGap},
	}
	for _, c := range cases {
		if got := angeDelay(c.since); got != c.want {
			t.Errorf("since %v: delay %v, want %v", c.since, got, c.want)
		}
	}
	if n := int(3 * time.Hour / tradeHistoryAngeGap); n > 12 {
		t.Errorf("%d merchant requests fit in 3 h, the budget is 12", n)
	}
}
