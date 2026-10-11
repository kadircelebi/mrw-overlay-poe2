package trade

import (
	"path/filepath"
	"testing"
	"time"

	"poe2filter/internal/prices"
)

func smartScanner(t *testing.T) *Scanner {
	t.Helper()
	s := NewScanner(NewClient("Forbidden Rites", 1), filepath.Join(t.TempDir(), "scan.json"), nil)
	s.SetRefresh(RefreshPolicy{Error: time.Hour, Empty: 72 * time.Hour, Hot: 12 * time.Hour, Normal: 24 * time.Hour})
	p := DefaultSmart
	s.SetSmart(&p)
	return s
}

func scanned(base string, kind prices.ExceptionalKind, min, listings, samples int, ex float64, at time.Time) *keyState {
	return &keyState{ExceptionalPrice: prices.ExceptionalPrice{Base: base, Kind: kind, Min: min, Listings: listings,
		Samples: samples, ValueEx: ex, ScannedAt: at}, LastAttempt: at, QueryVersion: scanQueryVersion}
}

func TestSmartIntervalHalvesWhenMovingAndDoublesWhenQuiet(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	old := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 40, 8, 100, now.Add(-13*time.Hour))
	s.mu.Lock()
	defer s.mu.Unlock()

	// 100 -> 140 ex (40 %): the 12 h valuable interval halves.
	moved := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 40, 8, 140, now)
	s.adapt(old, moved)
	if moved.IntervalMin != 6*60 {
		t.Fatalf("moving key interval %d min, want 360", moved.IntervalMin)
	}
	// Moving again and again stops at the 3 h floor.
	again := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 40, 8, 200, now)
	s.adapt(moved, again)
	third := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 40, 8, 300, now)
	s.adapt(again, third)
	if third.IntervalMin != 3*60 {
		t.Fatalf("floor %d min, want 180", third.IntervalMin)
	}
	// Quiet (300 -> 310 ex, 3 %): doubles back, never past the group's 12 h.
	quiet := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 41, 8, 310, now)
	s.adapt(third, quiet)
	if quiet.IntervalMin != 6*60 {
		t.Fatalf("quiet key interval %d min, want 360", quiet.IntervalMin)
	}
	calm := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 41, 8, 305, now)
	s.adapt(quiet, calm)
	calmer := scanned("Vile Robe", prices.KindQuality, ExceptionalQualityMin, 41, 8, 300, now)
	s.adapt(calm, calmer)
	if calmer.IntervalMin != 12*60 {
		t.Fatalf("calm key interval %d min, want the group's 720", calmer.IntervalMin)
	}
	if got := s.refreshAfter(third); got != 3*time.Hour {
		t.Fatalf("refreshAfter %v, want 3h", got)
	}
}

func TestSmartChangeNeedsEnoughListings(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range []struct {
		name         string
		aList, aSamp int
		aEx          float64
		bList, bSamp int
		bEx          float64
		want         bool
	}{
		{"lone listing price swing", 1, 1, 10, 1, 1, 50, false},
		{"one to two listings", 1, 1, 10, 2, 2, 10, false},
		{"listings appear", 0, 0, 0, 2, 2, 10, true},
		{"listings vanish", 5, 5, 10, 0, 0, 0, true},
		{"listings 10 -> 14", 10, 8, 10, 14, 8, 10, true},
		{"listings 40 -> 44", 40, 8, 10, 44, 8, 10, false},
		{"price -35 %", 20, 8, 100, 20, 8, 65, true},
		{"price +20 %", 20, 8, 100, 20, 8, 120, false},
	} {
		a := scanned("X", prices.KindSockets, 3, c.aList, c.aSamp, c.aEx, now)
		b := scanned("X", prices.KindSockets, 3, c.bList, c.bSamp, c.bEx, now)
		if got := s.moved(a.ExceptionalPrice, b.ExceptionalPrice); got != c.want {
			t.Errorf("%s: moved %v, want %v", c.name, got, c.want)
		}
	}
}

func TestChangedSearchStartsOverWithoutSignals(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	old := scanned("Vile Robe", prices.KindQuality, 21, 40, 8, 10, now.Add(-time.Hour))
	old.IntervalMin = 180
	now25 := scanned("Vile Robe", prices.KindQuality, 25, 10, 8, 500, now)
	s.adapt(old, now25)
	if now25.IntervalMin != 12*60 {
		t.Fatalf("new minimum kept %d min, want the group's 720", now25.IntervalMin)
	}
	if s.jumped(old, now25.ExceptionalPrice) {
		t.Fatal("a new quality minimum counted as a price jump")
	}
}

func TestPriceJumpBringsTheBaseForward(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.SetCandidates([]Candidate{{Base: "Vile Robe"}, {Base: "Zealot Bow"}})
	s.SetIlvlBuckets([]IlvlBucket{{Min: 79, Max: 81}, {Min: 82}})
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Classes["Vile Robe"], s.st.Classes["Zealot Bow"] = "Body Armours", "Bows"
	for _, base := range []string{"Vile Robe", "Zealot Bow"} {
		for _, b := range []string{"|ilvl79-81", "|ilvl82+"} {
			s.st.Keys[keyOf(base, prices.KindQuality)+b] = scanned(base, prices.KindQuality, ExceptionalQualityMin, 40, 8, 100, now.Add(-time.Hour))
			s.st.Keys[keyOf(base, prices.KindSockets)+b] = scanned(base, prices.KindSockets, 3, 40, 8, 100, now.Add(-time.Hour))
		}
	}
	if got := s.next(now); got != nil {
		t.Fatalf("nothing should be due, got %s", got.key)
	}

	key := "Vile Robe|sockets|ilvl82+"
	if !s.jumped(s.st.Keys[key], prices.ExceptionalPrice{Min: 3, Samples: 8, ValueEx: 200}) {
		t.Fatal("100 -> 200 ex is a jump")
	}
	if n := s.boostSiblings("Vile Robe", key); n != 3 {
		t.Fatalf("%d siblings brought forward, want 3", n)
	}
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		got := s.next(now)
		if got == nil || got.base != "Vile Robe" || got.key == key {
			t.Fatalf("pick %d = %+v, want another Vile Robe key", i, got)
		}
		seen[got.key] = true
		s.st.Keys[got.key] = scanned("Vile Robe", got.kind, got.min, 40, 8, 180, now)
	}
	if len(seen) != 3 || s.next(now) != nil {
		t.Fatalf("siblings %v, then %v", seen, s.next(now))
	}
}

func TestOverdueValuableKeysGoFirst(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.SetCandidates([]Candidate{{Base: "Cheap"}, {Base: "Dear"}, {Base: "Late"}})
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range []string{"Cheap", "Dear", "Late"} {
		s.st.Classes[b] = "Rings" // no socket kind: one key each
	}
	// Cheap: 25 h old at a 24 h interval. Dear: 13 h at 12 h, worth far more.
	// Late: cheap but 48 h old, twice its interval.
	s.st.Keys["Cheap|quality"] = scanned("Cheap", prices.KindQuality, ExceptionalQualityMin, 10, 5, 5, now.Add(-25*time.Hour))
	s.st.Keys["Dear|quality"] = scanned("Dear", prices.KindQuality, ExceptionalQualityMin, 10, 5, 2000, now.Add(-13*time.Hour))
	s.st.Keys["Late|quality"] = scanned("Late", prices.KindQuality, ExceptionalQualityMin, 10, 5, 5, now.Add(-49*time.Hour))
	var order []string
	for i := 0; i < 3; i++ {
		got := s.next(now)
		order = append(order, got.base)
		s.st.Keys[got.key].LastAttempt = now
	}
	if order[0] != "Dear" || order[1] != "Late" || order[2] != "Cheap" {
		t.Fatalf("order %v, want Dear, Late, Cheap", order)
	}
}

func TestEveryTenthSearchExploresTheOldestCheapKey(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.SetCandidates([]Candidate{{Base: "Due"}, {Base: "Empty"}, {Base: "Older"}, {Base: "Young"}})
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range []string{"Due", "Empty", "Older", "Young"} {
		s.st.Classes[b] = "Rings"
	}
	s.st.Keys["Due|quality"] = scanned("Due", prices.KindQuality, ExceptionalQualityMin, 10, 5, 500, now.Add(-13*time.Hour))
	s.st.Keys["Empty|quality"] = scanned("Empty", prices.KindQuality, ExceptionalQualityMin, 0, 0, 0, now.Add(-30*time.Hour))
	s.st.Keys["Older|quality"] = scanned("Older", prices.KindQuality, ExceptionalQualityMin, 0, 0, 0, now.Add(-50*time.Hour))
	s.st.Keys["Young|quality"] = scanned("Young", prices.KindQuality, ExceptionalQualityMin, 0, 0, 0, now.Add(-2*time.Hour))
	for i := 0; i < 9; i++ {
		s.searches = i
		if got := s.next(now); got == nil || got.base != "Due" {
			t.Fatalf("search %d picked %+v, want Due", i, got)
		}
	}
	s.searches = 9
	if got := s.next(now); got == nil || got.base != "Older" {
		t.Fatalf("exploring search picked %+v, want Older (empty, 50 h)", got)
	}
	// Exploring never adds searches: with nothing due the scanner rests.
	s.st.Keys["Due|quality"].LastAttempt = now
	if got := s.next(now); got != nil {
		t.Fatalf("idle scanner explored %s", got.key)
	}
}

func TestPaceSpacesSearches(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pace(now) != 0 {
		t.Fatal("first search waited")
	}
	s.lastSearch = now
	if got := s.pace(now.Add(30 * time.Second)); got != time.Minute {
		t.Fatalf("40/h after 30 s: wait %v, want 1m0s", got)
	}
	s.smart.PerHour = 60
	if got := s.pace(now.Add(30 * time.Second)); got != 30*time.Second {
		t.Fatalf("60/h after 30 s: wait %v, want 30s", got)
	}
	s.smart = nil
	if s.pace(now) != 0 {
		t.Fatal("the app's scanner is paced")
	}
}

func TestNewLeagueStartsWithLastLeaguesValuables(t *testing.T) {
	s := smartScanner(t)
	now := time.Now()
	s.SetCandidates([]Candidate{{Base: "Cheap"}, {Base: "Dear"}, {Base: "Dearer"}, {Base: "New"}, {Base: "Bow"}})
	s.SetPrior([]prices.ExceptionalPrice{
		{Base: "Cheap", Kind: prices.KindQuality, Min: 21, Listings: 5, ValueEx: 3, Class: "Rings"},
		{Base: "Dear", Kind: prices.KindQuality, Min: 21, Listings: 5, ValueEx: 300, Class: "Rings"},
		{Base: "Dearer", Kind: prices.KindQuality, Min: 21, Listings: 5, ValueEx: 3000, Class: "Rings"},
		{Base: "Bow", Kind: prices.KindQuality, Min: 21, Listings: 0, Class: "Bow"},
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	var order []string
	for {
		got := s.next(now)
		if got == nil {
			break
		}
		order = append(order, got.key)
		s.st.Keys[got.key] = scanned(got.base, got.kind, got.min, 1, 1, 1, now)
	}
	want := []string{"Dearer|quality", "Dear|quality", "New|quality", "New|sockets", "Bow|quality", "Bow|sockets", "Cheap|quality"}
	if len(order) != len(want) {
		t.Fatalf("order %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v", order, want)
		}
	}
	// The class learned last league ("Bow" from the trade site) spares the
	// two-socket probe: bows are searched at three.
	for _, k := range order {
		if k == "Bow|sockets" && s.st.Keys[k].Min != 3 {
			t.Fatalf("Bow sockets searched at %d", s.st.Keys[k].Min)
		}
	}
}

func TestImportOwnTakesOnlyThisMachinesKeys(t *testing.T) {
	weights := map[string]int{"office": 50, "home": 50}
	office, _ := ShardFilter("office", weights)
	src := NewScanner(NewClient("Forbidden Rites", 1), filepath.Join(t.TempDir(), "a.json"), nil)
	now := time.Now().UTC()
	var mine, theirs int
	for i := 0; i < 40; i++ {
		base := "Base " + string(rune('A'+i))
		key := keyOf(base, prices.KindQuality)
		src.st.Keys[key] = scanned(base, prices.KindQuality, 21, 5, 5, 50, now)
		if office(key) {
			mine++
		} else {
			theirs++
		}
	}
	raw, err := src.Export()
	if err != nil {
		t.Fatal(err)
	}
	dst := NewScanner(NewClient("Forbidden Rites", 1), filepath.Join(t.TempDir(), "b.json"), nil)
	dst.SetShard(office)
	res, err := dst.ImportOwn(raw)
	if err != nil || res.Added != mine || mine == 0 || theirs == 0 {
		t.Fatalf("added %d (err %v), want %d of %d", res.Added, err, mine, mine+theirs)
	}
	// Keys an earlier split left here are dropped.
	dst.mu.Lock()
	dst.st.Keys["Stray|quality"] = scanned("Stray", prices.KindQuality, 21, 5, 5, 50, now)
	stray := office("Stray|quality")
	dst.mu.Unlock()
	if n := dst.PruneForeign(); (n == 1) == stray {
		t.Fatalf("pruned %d, stray belongs here: %v", n, stray)
	}
}
