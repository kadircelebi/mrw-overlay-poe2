package trade

import (
	"net/http"
	"testing"
	"time"
)

func TestTrimmedMean(t *testing.T) {
	cases := []struct {
		in      []float64
		want    float64
		samples int
	}{
		{nil, 0, 0},
		{[]float64{10, 20}, 15, 2},
		{[]float64{1, 50, 50, 50, 50}, 50, 4},                   // one outlier dropped
		{[]float64{1, 2, 100, 100, 100, 100, 100, 100}, 100, 6}, // two dropped
	}
	for _, c := range cases {
		got, n := TrimmedMean(c.in)
		if got != c.want || n != c.samples {
			t.Errorf("%v: got %v/%d want %v/%d", c.in, got, n, c.want, c.samples)
		}
	}
}

func TestLimiterSpacingAndServerState(t *testing.T) {
	l := NewLimiter(0.5, parseRules("600:21600:3600"))
	now := time.Now()
	l.history = []time.Time{now}
	// 50% of 600 per 6h => one request every 72s.
	if d := l.nextSlot(now).Sub(now); d < 71*time.Second || d > 73*time.Second {
		t.Fatalf("spacing = %v, want ~72s", d)
	}

	// The user already burned the budget on the trade site: wait out the window.
	l2 := NewLimiter(0.5, nil)
	resp := &http.Response{StatusCode: 200, Header: http.Header{}}
	resp.Header.Set("X-Rate-Limit-Ip", "5:10:60,30:300:1800")
	resp.Header.Set("X-Rate-Limit-Ip-State", "1:10:0,20:300:0")
	l2.Observe(resp)
	if d := time.Until(l2.nextSlot(time.Now())); d < 290*time.Second {
		t.Fatalf("expected to wait for the 300s window, got %v", d)
	}

	// A 429 blocks for Retry-After.
	l3 := NewLimiter(0.5, parseRules("5:10:60"))
	r429 := &http.Response{StatusCode: 429, Header: http.Header{}}
	r429.Header.Set("Retry-After", "120")
	l3.Observe(r429)
	if d := time.Until(l3.nextSlot(time.Now())); d < 115*time.Second {
		t.Fatalf("expected ~120s block, got %v", d)
	}
}

func TestExceptionalSocketMin(t *testing.T) {
	if ExceptionalSocketMin("Boots") != 2 || ExceptionalSocketMin("Body Armours") != 3 || ExceptionalSocketMin("Rings") != 0 {
		t.Fatal("socket minimums wrong")
	}
}
