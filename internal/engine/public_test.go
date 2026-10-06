package engine

import (
	"testing"

	"poe2filter/internal/filter"
	"poe2filter/internal/publicprofile"
)

func sharedDoc(t *testing.T, minValue float64, waystone int) publicprofile.Document {
	t.Helper()
	c := filter.DefaultConfig()
	c.MinValue, c.WaystoneTier = minValue, waystone
	c.Whitelist = []string{"Divine Orb"}
	doc, _ := publicprofile.FromConfig(c)
	return doc
}

// Following someone's profile takes their filter settings and keeps the
// player's own; while it is active, filter edits are put back.
func TestFollowedProfileIsLockedToTheAuthor(t *testing.T) {
	e := newTestEngine(t)
	own := e.Config()
	own.LeagueName, own.FilterName, own.MinValue = "My League", "mine", 1
	if _, err := e.SetConfig(own); err != nil {
		t.Fatal(err)
	}

	l := publicprofile.Listing{ID: "abcdefghijkl", Name: "Expedition", Author: "Author#1234", Version: 1}
	name, err := e.AddFollowed(l, sharedDoc(t, 40, 12))
	if err != nil {
		t.Fatal(err)
	}
	if name != "Expedition · Author" {
		t.Fatalf("followed profile name: %q", name)
	}
	if again, _ := e.AddFollowed(l, sharedDoc(t, 40, 12)); again != name || len(e.Profiles()) != 2 {
		t.Fatalf("following twice made another profile: %q %+v", again, e.Profiles())
	}
	cfg, err := e.SwitchProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ = e.SetConfig(cfg)
	if cfg.MinValue != 40 || cfg.WaystoneTier != 12 || cfg.LeagueName != "My League" || cfg.FilterName != "mine" {
		t.Fatalf("followed settings: %+v", cfg)
	}

	cfg.MinValue, cfg.LeagueName = 999, "Other League"
	cfg, _ = e.SetConfig(cfg)
	if cfg.MinValue != 40 {
		t.Fatalf("an edit changed a followed profile's filter: %v", cfg.MinValue)
	}
	if cfg.LeagueName != "Other League" {
		t.Fatal("the player's own setting did not change")
	}

	if _, _, _, err := e.SharedProfile(name); err == nil {
		t.Fatal("a followed profile could be published")
	}

	// Save as gives an editable copy.
	if err := e.SaveProfileAs("My copy"); err != nil {
		t.Fatal(err)
	}
	cfg = e.Config()
	cfg.MinValue = 7
	if cfg, _ = e.SetConfig(cfg); cfg.MinValue != 7 {
		t.Fatal("the copy is still locked")
	}
}

func TestUpdateFollowed(t *testing.T) {
	e := newTestEngine(t)
	l := publicprofile.Listing{ID: "abcdefghijkl", Name: "Breach", Author: "A#0001", Version: 1}
	name, _ := e.AddFollowed(l, sharedDoc(t, 10, 5))

	// Not active: the stored config changes, nothing to apply.
	l.Version, l.Name = 2, "Breach v2"
	doc := sharedDoc(t, 20, 6)
	active, got, err := e.UpdateFollowed(l.ID, FollowedUpdate{Listing: l, Document: &doc})
	if err != nil || active != nil || got != name {
		t.Fatalf("inactive update: %v %v %q", active, err, got)
	}
	if v := e.FollowedVersions()[l.ID]; v != 2 {
		t.Fatalf("version not stored: %d", v)
	}

	// Active: the new config comes back to be applied.
	if _, err := e.SwitchProfile(name); err != nil {
		t.Fatal(err)
	}
	l.Version = 3
	doc = sharedDoc(t, 30, 7)
	active, _, err = e.UpdateFollowed(l.ID, FollowedUpdate{Listing: l, Document: &doc})
	if err != nil || active == nil || active.MinValue != 30 || active.WaystoneTier != 7 {
		t.Fatalf("active update: %+v %v", active, err)
	}

	// Removed on the server: marked, last copy kept.
	if _, _, err := e.UpdateFollowed(l.ID, FollowedUpdate{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range e.Profiles() {
		if p.Name == name && (p.Follow == nil || !p.Follow.Gone || p.Follow.Name != "Breach v2") {
			t.Fatalf("removed profile: %+v", p.Follow)
		}
	}

	// Deleting returns what the caller needs to stop following.
	if _, removed, err := e.DeleteProfile(name); err != nil || removed.Follow == nil || removed.Follow.ID != l.ID {
		t.Fatalf("delete: %+v %v", removed, err)
	}
	if len(e.FollowedVersions()) != 0 {
		t.Fatal("deleted profile still followed")
	}
}

func TestPublicIDIsKept(t *testing.T) {
	e := newTestEngine(t)
	name := e.Profiles()[0].Name
	if err := e.SetPublicID(name, "zyxwvutsrqpo"); err != nil {
		t.Fatal(err)
	}
	if err := e.RenameProfile(name, "Renamed"); err != nil {
		t.Fatal(err)
	}
	_, _, id, err := e.SharedProfile("Renamed")
	if err != nil || id != "zyxwvutsrqpo" {
		t.Fatalf("public id after rename: %q %v", id, err)
	}
}
