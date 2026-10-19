package lootlog

import (
	"testing"
	"time"
)

var (
	hideout = Area{ID: "HideoutShrine", Seed: "1", Rest: true}
	isle    = Area{ID: "MapWaywardIsle", Level: 79, Seed: "817"}
	arena   = Area{ID: "MapWaywardIsle_Boss", Level: 79, Seed: "55"}
	crypt   = Area{ID: "MapCrypt", Level: 80, Seed: "9"}
)

func worth(name string) float64 {
	return map[string]float64{"Divine Orb": 700, "Exalted Orb": 1, "Chaos Orb": 70}[name]
}

type clock struct{ t time.Time }

func (c *clock) next() time.Time { c.t = c.t.Add(time.Minute); return c.t }

func scan(at time.Time, items map[string]int) Scan { return Scan{At: at, Items: items} }

func TestARunIsTheGainBetweenTheReadsAroundIt(t *testing.T) {
	s := New(t.TempDir())
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	s.Place(hideout)
	if _, _, err := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 20, "Chaos Orb": 3}), worth); err != nil {
		t.Fatal(err)
	}
	_ = s.Entered("L", isle, c.next())
	_ = s.Entered("L", arena, c.next()) // the boss arena is part of the map
	_ = s.Entered("L", hideout, c.next())
	if id, at, ok := s.Waiting("L"); !ok || id != isle.ID || at.IsZero() {
		t.Fatalf("waiting %q %v", id, ok)
	}
	// Two more Exalted, a Divine, one Chaos used: the gain is 2 + 700.
	m, r, err := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 22, "Divine Orb": 1, "Chaos Orb": 2}), worth)
	if err != nil || m == nil || r == nil {
		t.Fatalf("no run closed: %v", err)
	}
	if !r.Fresh || r.NoBase || r.Unread || r.ValueEx != 702 || len(r.Items) != 2 || m.AreaID != isle.ID {
		t.Errorf("run %+v of %s", r, m.AreaID)
	}
	if _, _, ok := s.Waiting("L"); ok {
		t.Error("still waiting after the read")
	}
}

func TestBackIntoTheSameMapGoesOnWithIt(t *testing.T) {
	s := New(t.TempDir())
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	s.Place(hideout)
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 1}), worth)
	s.Entered("L", isle, c.next())
	s.Entered("L", hideout, c.next())
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 5}), worth) // +4, then stash it all
	s.Scanned("L", scan(c.next(), map[string]int{}), worth)
	s.Entered("L", isle, c.next()) // through the same portal
	s.Entered("L", hideout, c.next())
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 3}), worth)
	list := s.Maps("L")
	if len(list) != 1 || len(list[0].Runs) != 2 || list[0].ValueEx != 7 {
		t.Fatalf("maps %+v", list)
	}
	// Another instance of the same map is another map.
	s.Entered("L", Area{ID: isle.ID, Level: 79, Seed: "999"}, c.next())
	s.Entered("L", hideout, c.next())
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 4}), worth)
	if list = s.Maps("L"); len(list) != 2 || list[0].Seed != "999" || list[0].ValueEx != 1 {
		t.Fatalf("new instance: %+v", list)
	}
}

func TestUnreadBaselessAndStaleRuns(t *testing.T) {
	s := New(t.TempDir())
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	s.Place(hideout)
	// No read before the first map: its gain is not known.
	s.Entered("L", isle, c.next())
	s.Entered("L", hideout, c.next())
	if _, r, _ := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 5}), worth); r == nil || !r.NoBase || r.ValueEx != 0 {
		t.Fatalf("baseless run %+v", r)
	}
	// Home from the next map and straight into a third: no read after.
	s.Entered("L", crypt, c.next())
	s.Entered("L", hideout, c.next())
	s.Entered("L", isle, c.next())
	list := s.Maps("L")
	if len(list) != 2 || !list[0].Runs[0].Unread || list[0].AreaID != crypt.ID {
		t.Fatalf("unread run: %+v", list)
	}
	// That third map's base was read on an earlier visit home: stale.
	s.Entered("L", hideout, c.next())
	if _, r, _ := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 9}), worth); r == nil || r.Fresh || r.ValueEx != 4 {
		t.Fatalf("stale run %+v", r)
	}
}

func TestReadsInAMapKeepNothing(t *testing.T) {
	s := New(t.TempDir())
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	// Unknown place: nothing kept either.
	if m, r, _ := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 1}), worth); m != nil || r != nil {
		t.Fatal("kept a read made nowhere known")
	}
	s.Place(hideout)
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 1}), worth)
	s.Entered("L", isle, c.next())
	if m, _, _ := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 50}), worth); m != nil {
		t.Fatal("a read in the map closed a run")
	}
	s.Entered("L", hideout, c.next())
	if _, r, _ := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 3}), worth); r == nil || r.ValueEx != 2 {
		t.Fatalf("run %+v: the read in the map must not be the base", r)
	}
}

func TestTheLogSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	s.Place(hideout)
	s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 1}), worth)
	s.Entered("L", isle, c.next())
	// The app restarts while the player is in the map.
	s = New(dir)
	s.Place(isle)
	s.Entered("L", hideout, c.next())
	if _, r, _ := s.Scanned("L", scan(c.next(), map[string]int{"Exalted Orb": 2}), worth); r == nil || r.ValueEx != 1 {
		t.Fatalf("run across a restart: %+v", r)
	}
	if list := New(dir).Maps("L"); len(list) != 1 {
		t.Fatalf("maps after reopening: %+v", list)
	}
	now := time.Date(2026, 10, 19, 20, 3, 0, 0, time.UTC)
	if tot := Sum(s.Maps("L"), now); tot.Maps != 0 {
		t.Errorf("sum since after the map: %+v", tot)
	}
	if err := s.Delete("L", s.Maps("L")[0].At); err != nil || len(s.Maps("L")) != 0 {
		t.Errorf("delete: %v %+v", err, s.Maps("L"))
	}
}

func TestGainCountsOnlyWhatWentUp(t *testing.T) {
	got := Gain(map[string]int{"A": 5, "B": 2}, map[string]int{"A": 3, "B": 4, "C": 1})
	if len(got) != 2 || got[0] != (Item{Name: "B", Count: 2}) || got[1] != (Item{Name: "C", Count: 1}) {
		t.Errorf("gain %+v", got)
	}
}

func TestStateFollowsTheTripAndTheFarm(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	if st := s.State("L"); st.Known || st.Last != nil {
		t.Fatalf("fresh store %+v", st)
	}
	s.Place(hideout)
	read := c.next()
	s.Scanned("L", scan(read, map[string]int{"Exalted Orb": 3}), worth)
	if st := s.State("L"); !st.InRest || st.Last == nil || !st.Last.At.Equal(read) || !st.Fresh || st.TripArea != "" {
		t.Fatalf("in the hideout %+v", st)
	}
	in := c.next()
	s.Entered("L", isle, in)
	st := s.State("L")
	if st.InRest || st.TripArea != isle.ID || !st.Entered.Equal(in) || st.Base == nil || st.Base.Items["Exalted Orb"] != 3 || !st.TripFresh || st.Back {
		t.Fatalf("in the map %+v", st)
	}
	s.Entered("L", hideout, c.next())
	if st := s.State("L"); !st.Back || st.Fresh {
		t.Fatalf("back %+v", st)
	}
	farm := c.next()
	if err := s.StartFarm("L", farm); err != nil {
		t.Fatal(err)
	}
	if st := New(dir).State("L"); !st.Farm.Equal(farm) {
		t.Errorf("farm start not kept: %v", st.Farm)
	}
}

func TestARunKeepsTheDoubtsOfBothReads(t *testing.T) {
	s := New(t.TempDir())
	c := &clock{time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)}
	s.Place(hideout)
	before := scan(c.next(), map[string]int{"Exalted Orb": 3})
	before.Unsure, before.Doubts = 1, []Doubt{{Row: 2, Col: 7, Name: "Exalted Orb", Count: 3, Why: "count"}}
	s.Scanned("L", before, worth)
	s.Entered("L", isle, c.next())
	s.Entered("L", hideout, c.next())
	after := scan(c.next(), map[string]int{"Exalted Orb": 5})
	after.Unsure, after.Doubts = 1, []Doubt{{Row: 1, Col: 1, Name: "Uncut Skill Gem", Count: 1, Why: "item"}}
	_, r, _ := s.Scanned("L", after, worth)
	if r == nil || r.Unsure != 2 || len(r.Doubts) != 2 || r.Doubts[0].After || !r.Doubts[1].After || r.Doubts[1].Why != "item" {
		t.Fatalf("run %+v", r)
	}
}
