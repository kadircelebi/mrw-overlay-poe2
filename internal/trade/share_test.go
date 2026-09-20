package trade

import (
	"path/filepath"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

func newTestScanner(t *testing.T, league string) *Scanner {
	t.Helper()
	return NewScanner(NewClient(league, 0.4), filepath.Join(t.TempDir(), "scan.json"), nil)
}

func priced(base string, ex float64, at time.Time) *keyState {
	return &keyState{ExceptionalPrice: prices.ExceptionalPrice{
		Base: base, Class: "Helmets", Kind: prices.KindSockets, Min: 2,
		ValueEx: ex, Listings: 20, Samples: 5, ScannedAt: at,
	}}
}

// A scan costs hours of rate-limited searches, so the results travel between
// players as a file. The merge must never move anyone backwards.
func TestScanShareRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	giver := newTestScanner(t, "Forbidden Rites")
	giver.st.Classes["Warded Helm"] = "Helmets"
	giver.st.Keys["Warded Helm|sockets"] = priced("Warded Helm", 7.5, now)
	giver.st.Keys["Felt Cap|sockets"] = priced("Felt Cap", 1, now)
	giver.st.Keys["Broken|sockets"] = &keyState{LastError: "HTTP 429"} // never scanned

	data, err := giver.Export()
	if err != nil {
		t.Fatal(err)
	}

	taker := newTestScanner(t, "Forbidden Rites")
	// The taker already has a fresher price for one of them.
	taker.st.Keys["Felt Cap|sockets"] = priced("Felt Cap", 3, now.Add(time.Hour))

	res, err := taker.Import(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Updated != 0 || res.Skipped != 1 {
		t.Errorf("merge counts: %+v", res)
	}
	if got := taker.st.Keys["Warded Helm|sockets"]; got == nil || got.ValueEx != 7.5 {
		t.Errorf("imported price missing: %+v", got)
	}
	if got := taker.st.Keys["Felt Cap|sockets"]; got == nil || got.ValueEx != 3 {
		t.Error("an older imported price must not overwrite a fresher local one")
	}
	if _, ok := taker.st.Keys["Broken|sockets"]; ok {
		t.Error("a key that was never scanned carries nothing worth sharing")
	}
	if taker.st.Classes["Warded Helm"] != "Helmets" {
		t.Error("the class of an imported base should come along")
	}
}

// Prices are league specific: importing across leagues would quietly poison the
// filter with values from a different economy.
func TestScanShareRefusesOtherLeague(t *testing.T) {
	giver := newTestScanner(t, "Standard")
	giver.st.Keys["Felt Cap|sockets"] = priced("Felt Cap", 1, time.Now())
	data, err := giver.Export()
	if err != nil {
		t.Fatal(err)
	}
	taker := newTestScanner(t, "Forbidden Rites")
	if _, err := taker.Import(data); err == nil {
		t.Fatal("expected the import to be refused")
	}
	if len(taker.st.Keys) != 0 {
		t.Error("a refused import must not change anything")
	}
}

func TestScanShareRejectsJunk(t *testing.T) {
	taker := newTestScanner(t, "Forbidden Rites")
	if _, err := taker.Import([]byte("this is not json")); err == nil {
		t.Error("expected an error for a file that is not an export")
	}
	if _, err := taker.Import([]byte(`{"version": 99, "league": "Forbidden Rites"}`)); err == nil {
		t.Error("expected an error for a newer file format")
	}
}
