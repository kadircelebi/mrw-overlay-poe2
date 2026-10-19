package lootlog

import (
	"testing"
	"time"
)

func TestSessionsFollowTheLogAndTheGameStarts(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	t0 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	enter := func(min int, a Area) Line { return Line{Mark: Mark{At: at(min), Area: &a}} }
	pos := func(off int64, min int) LogPos { return LogPos{Path: "Client.txt", Offset: off, Last: at(min)} }

	// The game starts, the player goes into a map and back.
	if err := s.Follow("L", []Line{{Mark: Mark{At: at(0)}, Open: true}, enter(1, hideout), enter(3, isle)}, pos(100, 3), true); err != nil {
		t.Fatal(err)
	}
	s.Follow("L", []Line{enter(8, hideout)}, pos(200, 8), false)
	// A crash: back in 5 minutes, the same session; the 5 minutes count
	// for nothing.
	s.Follow("L", []Line{{Mark: Mark{At: at(15)}, Open: true, Prev: at(10)}, enter(16, hideout), enter(17, crypt)}, pos(300, 17), false)
	got := New(dir).Sessions("L") // kept on disk
	if len(got) != 1 || !got[0].Start.Equal(at(0)) || !got[0].Open() {
		t.Fatalf("one open session: %+v", got)
	}
	tm, sum := got[0].Clock(at(20))
	if tm.InMaps != 5*time.Minute+3*time.Minute || tm.InRest != 2*time.Minute+2*time.Minute+time.Minute || sum.Maps != 2 {
		t.Errorf("times %+v sum %+v", tm, sum)
	}
	if p := New(dir).LogPos(); p.Offset != 300 || !p.Last.Equal(at(17)) {
		t.Errorf("log pos %+v", p)
	}

	// Closed for an hour or more: a new session; the old one keeps its
	// times to the log's last line before.
	s.Follow("L", []Line{{Mark: Mark{At: at(90)}, Open: true, Prev: at(25)}, enter(91, hideout)}, pos(400, 91), false)
	got = s.Sessions("L")
	if len(got) != 2 || got[0].Open() || !got[0].End.Equal(at(25)) || got[0].Marks != nil || !got[1].Start.Equal(at(90)) {
		t.Fatalf("two sessions: %+v", got)
	}
	if want := (Summary{Active: 18 * 60, InMaps: 13 * 60, InRest: 5 * 60, Maps: 2}); got[0].Times != want {
		t.Errorf("closed times %+v, want %+v", got[0].Times, want)
	}

	// Read again from the session's start (Again): what was had stays once.
	s.Follow("L", []Line{{Mark: Mark{At: at(90)}, Open: true, Prev: at(91)}, enter(91, hideout), enter(95, isle)}, pos(500, 95), true)
	if got := s.Sessions("L"); len(got) != 2 || len(got[1].Marks) != 2 || got[1].Marks[1].Area.ID != isle.ID {
		t.Fatalf("read again: %+v", got)
	}
}

func TestANewSessionGoesOnFromWhereThePlayerIs(t *testing.T) {
	s := New(t.TempDir())
	t0 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	s.Follow("L", []Line{{Mark: Mark{At: at(0), Area: &isle}}, {Mark: Mark{At: at(1), AFK: true}}}, LogPos{}, false)
	if err := s.NewSession("L", at(4)); err != nil {
		t.Fatal(err)
	}
	got := s.Sessions("L")
	if len(got) != 2 || !got[0].End.Equal(at(4)) || got[0].Times.InMaps != 4*60 {
		t.Fatalf("sessions %+v", got)
	}
	if m := got[1].Marks; len(m) != 2 || m[0].Area.ID != isle.ID || !m[0].At.Equal(at(4)) || !m[1].AFK {
		t.Fatalf("the new one's marks %+v", m)
	}
	if tm, _ := got[1].Clock(at(6)); tm.InMaps != 2*time.Minute || tm.Live != LiveMap {
		t.Errorf("the new one in the isle: %+v", tm)
	}
}

func TestMapsGoInTheirSessionsAndGoWithThem(t *testing.T) {
	s := New(t.TempDir())
	t0 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	// A map before any session, then sessions at 10 and 20.
	s.Place(hideout)
	run := func(min int, a Area) {
		s.Scanned("L", scan(at(min), map[string]int{}), worth)
		s.Entered("L", a, at(min+1))
		s.Entered("L", hideout, at(min+2))
		s.Scanned("L", scan(at(min+3), map[string]int{"Exalted Orb": min}), worth)
	}
	run(0, isle)
	s.NewSession("L", at(10))
	run(11, crypt)
	s.NewSession("L", at(20))
	run(21, isle)
	run(25, crypt)
	sessions := s.Sessions("L")
	by, before := Group(sessions, s.Maps("L"))
	if len(before) != 1 || len(by) != 2 || len(by[0]) != 1 || len(by[1]) != 2 || by[1][0].AreaID != crypt.ID {
		t.Fatalf("groups %v / %v", by, before)
	}

	if ok, _ := s.SetSessionCost("L", sessions[0].ID(), &Cost{ValueEx: 50}); !ok {
		t.Fatal("cost not set")
	}
	s.RenameSession("L", sessions[1].ID(), "  Breach farm  ")
	if ok, _ := s.MergeSession("L", sessions[1].ID()); !ok {
		t.Fatal("not merged")
	}
	got := s.Sessions("L")
	if len(got) != 1 || !got[0].Open() || got[0].Name != "Breach farm" || got[0].Cost == nil || got[0].Cost.ValueEx != 50 {
		t.Fatalf("merged %+v", got)
	}
	if ok, _ := s.MergeSession("L", got[0].ID()); ok {
		t.Error("the first session merged into nothing")
	}

	if ok, _ := s.DeleteSession("L", got[0].ID(), at(40)); !ok {
		t.Fatal("not deleted")
	}
	got = s.Sessions("L")
	if maps := s.Maps("L"); len(maps) != 1 || maps[0].AreaID != isle.ID || !maps[0].At.Equal(at(1)) {
		t.Errorf("maps left %+v", maps)
	}
	if len(got) != 1 || !got[0].Open() || !got[0].Start.Equal(at(40)) || got[0].Name != "" {
		t.Errorf("the open session begins again: %+v", got)
	}
}
