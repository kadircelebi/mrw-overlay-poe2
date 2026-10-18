package expeditionhistory

import (
	"testing"
	"time"
)

func TestChooseAndEnter(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.Local)
	a := Entry{At: at, AreaID: "MapWaywardIsle", Seed: "1", Name: "Chaos Orb", ValueEx: 70}
	if err := s.Choose("L", a); err != nil {
		t.Fatal(err)
	}
	// A map gives several rewards: a second choice there is kept too.
	b := a
	b.Name, b.ValueEx = "Divine Orb", 700
	if err := s.Choose("L", b); err != nil {
		t.Fatal(err)
	}
	if list := s.Entries("L"); len(list) != 2 || list[0].Name != "Divine Orb" || list[1].Done {
		t.Fatalf("after second choice %+v", list)
	}
	// A trip home and back to the same map closes nothing.
	if changed, _ := s.Entered("L", "HideoutShrine", "1", true); changed {
		t.Error("hideout closed the map")
	}
	if changed, _ := s.Entered("L", "MapWaywardIsle", "1", false); changed {
		t.Error("the same map closed itself")
	}
	if p, ok := s.Pending("L"); !ok || p.Name != "Divine Orb" {
		t.Fatalf("pending %+v %v", p, ok)
	}
	// The same map id with another seed is another map.
	if changed, err := s.Entered("L", "MapWaywardIsle", "2", false); !changed || err != nil {
		t.Fatalf("next map %v %v", changed, err)
	}
	if _, ok := s.Pending("L"); ok {
		t.Error("still pending after the next map")
	}
	// A choice in another map without entering it first closes the old one.
	c := Entry{At: at.Add(time.Hour), AreaID: "MapCrypt", Seed: "9", Name: "Exalted Orb", ValueEx: 1}
	_ = s.Choose("L", Entry{At: at.Add(30 * time.Minute), AreaID: "MapRiver", Seed: "5", ValueEx: 5})
	_ = s.Choose("L", c)
	list := New(dir).Entries("L") // read back from the file
	if len(list) != 4 || list[0].Name != "Exalted Orb" || list[0].Done || !list[1].Done || !list[2].Done || !list[3].Done {
		t.Fatalf("history %+v", list)
	}
	if tot := Open(list); tot.Maps != 1 || tot.ValueEx != 1 {
		t.Errorf("open %+v", tot)
	}
	// Every choice counts, the pending one too.
	if tot := Sum(list, time.Time{}); tot.Maps != 4 || tot.ValueEx != 776 {
		t.Errorf("sum %+v", tot)
	}
	if tot := Sum(list, at.Add(time.Minute)); tot.Maps != 2 || tot.ValueEx != 6 {
		t.Errorf("sum since %+v", tot)
	}
	if err := s.Clear(); err != nil || len(s.Entries("L")) != 0 {
		t.Errorf("clear %v", err)
	}
}
