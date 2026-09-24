package trade

import (
	"encoding/json"
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

func TestInteractiveLimiterUsesPermittedBurst(t *testing.T) {
	l := NewLimiter(0.5, parseRules("5:10:60,600:21600:3600"))
	l.SetEvenPacing(false)
	now := time.Now()
	l.history = []time.Time{now}
	if slot := l.nextSlot(now); slot.After(now) {
		t.Fatalf("one interactive request should not create a 6-hour-window pacing delay: %v", slot.Sub(now))
	}

	// 50% of the 5/10s rule permits two immediate requests, then waits for
	// that short window. The long-window cap is still enforced by nextSlot.
	l.history = append(l.history, now)
	if d := l.nextSlot(now).Sub(now); d < 9*time.Second || d > 11*time.Second {
		t.Fatalf("short burst limit = %v, want ~10s", d)
	}
}

func TestExceptionalSocketMin(t *testing.T) {
	if ExceptionalSocketMin("Boots") != 2 || ExceptionalSocketMin("Body Armours") != 3 || ExceptionalSocketMin("Rings") != 0 {
		t.Fatal("socket minimums wrong")
	}
}

func TestFetchListingCarriesTravelAndItemState(t *testing.T) {
	raw := []byte(`{"result":[{"id":"item-1","listing":{"hideout_token":"signed-item-token"},"item":{"identified":false,"fractured":true,"corrupted":true,"sanctified":true}}]}`)
	var response evaluatedFetchResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Result) != 1 || response.Result[0].Listing.HideoutToken != "signed-item-token" {
		t.Fatalf("hideout token not decoded: %+v", response.Result)
	}
	if !response.Result[0].Item.Fractured || !response.Result[0].Item.Corrupted || !response.Result[0].Item.Sanctified {
		t.Fatalf("item states not decoded: %+v", response.Result[0].Item)
	}
	if response.Result[0].Item.Identified {
		t.Fatalf("unidentified state not decoded: %+v", response.Result[0].Item)
	}
}

func TestFormatTradeProperty(t *testing.T) {
	cases := []struct {
		name      string
		values    [][]interface{}
		wantName  string
		wantValue string
	}{
		{"Recovers {0} Life over {1} Seconds", [][]interface{}{{"920", 0}, {"3", 0}}, "Recovers 920 Life over 3 Seconds", ""},
		{"Consumes {0} of {1} Charges on use", [][]interface{}{{"22", 0}, {"75", 0}}, "Consumes 22 of 75 Charges on use", ""},
		{"[Quality]", [][]interface{}{{"+20%", 1}}, "Quality", "+20%"},
		{"[Flask|Kalguuran Flask]", nil, "Kalguuran Flask", ""},
		{"Elemental Damage", [][]interface{}{{"4-233", 6}, {"1-43", 5}}, "Elemental Damage", "4-233, 1-43"},
	}
	for _, c := range cases {
		name, value := formatTradeProperty(c.name, c.values)
		if name != c.wantName || value != c.wantValue {
			t.Errorf("%q → %q / %q, want %q / %q", c.name, name, value, c.wantName, c.wantValue)
		}
	}
}

// The expected numbers are the listing's own weapon values as the trade site
// shows them: average hit times attacks per second.
func TestAddWeaponDPS(t *testing.T) {
	item := EvaluatedItem{Properties: []EvaluatedProperty{
		{Name: "Physical Damage", Value: "47-188"},
		{Name: "Elemental Damage", Value: "118-185, 5-109"},
		{Name: "Attacks per Second", Value: "1.65"},
	}}
	addWeaponDPS(&item)
	if item.PhysicalDPS != 193.88 || item.ElementalDPS != 344.03 || item.DPS != 537.9 {
		t.Fatalf("pdps=%v edps=%v dps=%v", item.PhysicalDPS, item.ElementalDPS, item.DPS)
	}
	armour := EvaluatedItem{Properties: []EvaluatedProperty{{Name: "Armour", Value: "934"}}}
	addWeaponDPS(&armour)
	if armour.DPS != 0 || len(armour.Properties) != 1 {
		t.Fatalf("non-weapon got DPS: %+v", armour)
	}
}
