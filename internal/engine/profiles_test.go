package engine

import (
	"strings"
	"testing"

	"poe2filter/internal/filter"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	return New(Options{Dir: t.TempDir()})
}

// Switching profiles is the point of the feature, so each one has to keep its
// own settings, including the edits made just before leaving it.
func TestProfilesKeepTheirOwnSettings(t *testing.T) {
	e := newTestEngine(t)

	first := e.Profiles()
	if len(first) != 1 || !first[0].Active {
		t.Fatalf("a fresh install should have one active profile, got %+v", first)
	}
	base := first[0].Name

	cfg := e.Config()
	cfg.MinValue, cfg.Strictness = 50, 6
	if _, err := e.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := e.SaveProfileAs("Breach"); err != nil {
		t.Fatal(err)
	}
	// Now edit only the new profile.
	cfg = e.Config()
	cfg.MinValue, cfg.Strictness = 5, 2
	if _, err := e.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	back, err := e.SwitchProfile(base)
	if err != nil {
		t.Fatal(err)
	}
	if back.MinValue != 50 || back.Strictness != 6 {
		t.Errorf("the first profile lost its settings: %.0f / %d", back.MinValue, back.Strictness)
	}
	if _, err := e.SetConfig(back); err != nil {
		t.Fatal(err)
	}

	breach, err := e.SwitchProfile("Breach")
	if err != nil {
		t.Fatal(err)
	}
	if breach.MinValue != 5 || breach.Strictness != 2 {
		t.Errorf("the edits made in Breach were lost: %.0f / %d", breach.MinValue, breach.Strictness)
	}
}

func TestProfileExportImport(t *testing.T) {
	giver := newTestEngine(t)
	cfg := giver.Config()
	cfg.MinValue, cfg.FilterName = 123, "simulacrum"
	cfg.ItemGroups = []filter.ItemGroup{{
		ID: "g1", Name: "One Divine", Mode: filter.ItemGroupModeValue,
		ThresholdValue: 1, ThresholdUnit: "divine",
	}}
	if _, err := giver.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := giver.SaveProfileAs("Simulacrum"); err != nil {
		t.Fatal(err)
	}
	data, err := giver.ExportProfile("Simulacrum")
	if err != nil {
		t.Fatal(err)
	}

	taker := newTestEngine(t)
	name, err := taker.ImportProfile(data)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Simulacrum" {
		t.Errorf("imported under %q", name)
	}
	got, err := taker.SwitchProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	if got.MinValue != 123 || got.FilterName != "simulacrum" {
		t.Errorf("imported settings wrong: %.0f %q", got.MinValue, got.FilterName)
	}
	if len(got.ItemGroups) != 1 || got.ItemGroups[0].GroupMode() != filter.ItemGroupModeValue ||
		got.ItemGroups[0].ThresholdValue != 1 || got.ItemGroups[0].ThresholdUnit != "divine" {
		t.Errorf("value group did not survive profile export/import: %+v", got.ItemGroups)
	}

	// Importing the same file again must not overwrite the first copy.
	second, err := taker.ImportProfile(data)
	if err != nil {
		t.Fatal(err)
	}
	if second == name {
		t.Error("a clashing name should be made unique")
	}
	if n := len(taker.Profiles()); n != 3 {
		t.Errorf("expected the default plus two imports, got %d", n)
	}
}

func TestProfileGuards(t *testing.T) {
	e := newTestEngine(t)
	if err := e.SaveProfileAs("  "); err == nil {
		t.Error("a nameless profile should be refused")
	}
	if _, err := e.SwitchProfile("nope"); err == nil {
		t.Error("switching to a missing profile should fail")
	}
	if _, err := e.DeleteProfile(e.Profiles()[0].Name); err == nil {
		t.Error("the last profile must not be deletable")
	}
	if _, err := e.ImportProfile([]byte("not json")); err == nil {
		t.Error("a broken file should be refused")
	}
	if _, err := e.ImportProfile([]byte(`{"version": 99}`)); err == nil {
		t.Error("a newer format should be refused")
	}

	// Deleting the active profile falls back to another one.
	if err := e.SaveProfileAs("Second"); err != nil {
		t.Fatal(err)
	}
	active, err := e.DeleteProfile("Second")
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(active, "Second") {
		t.Error("the deleted profile is still active")
	}
	if n := len(e.Profiles()); n != 1 {
		t.Errorf("expected one profile left, got %d", n)
	}
}

func TestRenameProfile(t *testing.T) {
	e := newTestEngine(t)
	first := e.Profiles()[0].Name
	if err := e.SaveProfileAs("Breach"); err != nil {
		t.Fatal(err)
	}
	cfg := e.Config()
	cfg.MinValue = 321
	if _, err := e.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := e.RenameProfile("Breach", "  Simulacrum  "); err != nil {
		t.Fatal(err)
	}
	profiles := e.Profiles()
	if len(profiles) != 2 || profiles[1].Name != "Simulacrum" || !profiles[1].Active {
		t.Fatalf("renamed profile state is wrong: %+v", profiles)
	}
	if _, err := e.SwitchProfile("Breach"); err == nil {
		t.Error("old profile name still resolves")
	}
	got, err := e.SwitchProfile("Simulacrum")
	if err != nil || got.MinValue != 321 {
		t.Fatalf("renamed profile lost its settings: %.0f, %v", got.MinValue, err)
	}
	if err := e.RenameProfile("Simulacrum", first); err == nil {
		t.Error("renaming over another profile should fail")
	}
	if err := e.RenameProfile("Simulacrum", "  "); err == nil {
		t.Error("renaming to an empty name should fail")
	}
	if err := e.RenameProfile("Simulacrum", "SIMULACRUM"); err != nil {
		t.Fatalf("case-only rename should work: %v", err)
	}
}
