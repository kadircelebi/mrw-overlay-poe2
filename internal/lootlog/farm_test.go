package lootlog

import (
	"testing"
	"time"
)

func TestFarmTimesCountMapsHomeAndBreaks(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	at := func(min, sec int) time.Time {
		return t0.Add(time.Duration(min)*time.Minute + time.Duration(sec)*time.Second)
	}
	enter := func(min, sec int, a Area) Mark { return Mark{At: at(min, sec), Area: &a} }
	marks := []Mark{
		enter(0, 0, hideout),
		enter(2, 0, isle),     // 2:00 home
		enter(5, 0, arena),    // the boss arena is part of the isle
		enter(6, 30, hideout), // 4:30 in the isle
		{At: at(7, 0), AFK: true},
		{At: at(9, 0)},        // 2:00 away
		enter(10, 0, isle),    // back through the portal: 3:30 home, 1:30 of it counted
		enter(11, 0, hideout), // the isle again: 5:30 in all
		enter(30, 0, crypt),   // 19:00 home: 10:00 count, 9:00 a break
	}
	got := FarmTimes(marks, t0, at(33, 0)) // 3:00 into the crypt
	want := Times{
		InMaps: 5*time.Minute + 30*time.Second + 3*time.Minute,
		InRest: 2*time.Minute + 90*time.Second + 10*time.Minute,
		Breaks: 2*time.Minute + 9*time.Minute,
		Live:   LiveMap, Since: at(30, 0),
	}
	want.Active = want.InMaps + want.InRest
	if got.InMaps != want.InMaps || got.InRest != want.InRest || got.Breaks != want.Breaks || got.Active != want.Active || got.Live != want.Live || !got.Since.Equal(want.Since) {
		t.Errorf("times %+v, want %+v", got, want)
	}
	if len(got.Maps) != 2 || got.Maps[0].AreaID != isle.ID || got.Maps[0].Dur != 5*time.Minute+30*time.Second || !got.Maps[0].At.Equal(at(2, 0)) || got.Maps[1].Dur != 3*time.Minute {
		t.Errorf("maps %+v", got.Maps)
	}

	// From the second stay on: the isle's first trip is cut off.
	got = FarmTimes(marks, at(8, 0), at(12, 0))
	if got.InMaps != time.Minute || got.InRest != 2*time.Minute || len(got.Maps) != 1 || got.Maps[0].Dur != time.Minute || got.Live != LiveRest || got.RestLeft != 9*time.Minute {
		t.Errorf("since 9:08: %+v", got)
	}

	// Home past the limit: a break.
	if got := FarmTimes(marks[:8], t0, at(25, 0)); got.Live != LiveBreak || got.InRest != 2*time.Minute+90*time.Second+10*time.Minute {
		t.Errorf("long stay: %+v", got)
	}
}

func TestMapTimesAreKeptWithTheirMaps(t *testing.T) {
	s := New(t.TempDir())
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	s.Place(hideout)
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 1}), worth)
	in := c.next()
	s.Entered("L", isle, in)
	s.Entered("L", hideout, c.next())
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 3}), worth)
	times := []MapTime{
		{AreaID: isle.ID, Seed: isle.Seed, At: in.Add(-3 * time.Second), Dur: 272 * time.Second},
		{AreaID: crypt.ID, Seed: crypt.Seed, At: in, Dur: time.Minute}, // not in the log
	}
	if changed, err := s.SetMapTimes("L", times); err != nil || !changed {
		t.Fatalf("changed %v, %v", changed, err)
	}
	if m := s.Maps("L"); len(m) != 1 || m[0].Secs != 272 {
		t.Fatalf("maps %+v", m)
	}
	if changed, _ := s.SetMapTimes("L", times); changed {
		t.Error("the same times again changed something")
	}
}
