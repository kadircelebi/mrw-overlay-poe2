package campaign

import (
	"regexp"
	"testing"
	"unicode/utf8"
)

// The ids the game logs for the campaign's areas (from a character's whole
// campaign, interludes and side areas included).
var loggedAreas = []string{
	"G1_1", "G1_town", "G1_2", "G1_3", "G1_4", "G1_5", "G1_6", "G1_7", "G1_8", "G1_9", "G1_11", "G1_12", "G1_13_1", "G1_13_2", "G1_14", "G1_15",
	"G2_1", "G2_town", "G2_10_1", "G2_10_2", "G2_2", "G2_13", "G2_3", "G2_3a", "G2_4_1", "G2_4_2", "G2_4_3", "G2_5_1", "Abyss_Intro", "G2_5_2", "G2_6", "G2_7", "G2_8", "G2_9_1", "G2_9_2", "G2_12",
	"G3_1", "G3_town", "G3_3", "G3_2_1", "G3_7", "G3_4", "G3_5", "G3_10_Airlock", "G3_10", "G3_6_1", "G3_6_2", "G3_2_2", "G3_8", "G3_9", "G3_11", "G3_12", "G3_14", "G3_16", "G3_17",
	"G4_town", "G4_2_1", "G4_2_2", "G4_3_1", "G4_3_2", "G4_5_1", "G4_5_2", "G4_1_1", "G4_1_2", "G4_7", "G4_4_1", "G4_4_2", "G4_4_3", "G4_8b", "G4_10", "G4_11_1b", "G4_11_2", "G4_13",
	"P1_Town", "P1_1", "P1_2", "P1_3", "P1_4", "P1_5", "P1_6",
	"P2_Town", "P2_1", "P2_2", "P2_3", "P2_5", "P2_6", "P2_7",
	"P3_Town", "P3_1", "P3_2", "P3_3", "P3_4", "P3_5", "P3_6", "P3_7",
}

func TestEveryLoggedAreaIsInTheGuide(t *testing.T) {
	for _, id := range loggedAreas {
		if _, _, ok := Find(id); !ok {
			t.Errorf("no notes for %s", id)
		}
	}
	for _, id := range []string{"MapWaywardIsle", "HideoutShrine", "G_Endgame_Town", "Delirium_Act1Town", ""} {
		if _, _, ok := Find(id); ok {
			t.Errorf("%s is not a campaign area", id)
		}
	}
}

func TestFindTakesAnAreasOtherIds(t *testing.T) {
	a, act, ok := Find("G2_3a")
	if !ok || a.ID != "G2_3" || act.ID != "act2" {
		t.Fatalf("G2_3a: %v %v %v", a.ID, act.ID, ok)
	}
}

// Words per line at the box's width, kept low so that real text (wider or
// narrower letters, words that do not break) still fits.
const lineChars = 44

// The lines the box has room for under its title.
const boxLines = 16

func noteLines(s string) int { return (utf8.RuneCountInString(s) + lineChars - 1) / lineChars }

func TestNotesAreCompleteAndFitTheBox(t *testing.T) {
	seen := map[string]bool{}
	kinds := map[string]bool{KindMust: true, KindOptional: true, KindTip: true, KindNext: true}
	mark := regexp.MustCompile(`[<>{}\[\]]`)
	for _, act := range Load().Acts {
		if act.ID == "" || act.EN == "" || act.TR == "" || len(act.Areas) == 0 {
			t.Errorf("act %q is incomplete", act.ID)
		}
		for _, a := range act.Areas {
			for _, id := range append([]string{a.ID}, a.Also...) {
				if seen[id] {
					t.Errorf("%s listed twice", id)
				}
				seen[id] = true
			}
			if a.Name == "" || len(a.Notes) == 0 {
				t.Errorf("%s is incomplete", a.ID)
			}
			en, tr := 0, 0
			for _, n := range a.Notes {
				if !kinds[n.Kind] {
					t.Errorf("%s: kind %q", a.ID, n.Kind)
				}
				if n.EN == "" || n.TR == "" {
					t.Errorf("%s: a note lacks a language: %q / %q", a.ID, n.EN, n.TR)
				}
				if mark.MatchString(n.EN) || mark.MatchString(n.TR) {
					t.Errorf("%s: markup in %q / %q", a.ID, n.EN, n.TR)
				}
				en += noteLines(n.EN)
				tr += noteLines(n.TR)
			}
			if en > boxLines || tr > boxLines {
				t.Errorf("%s: %d/%d lines do not fit the box's %d", a.ID, en, tr, boxLines)
			}
		}
	}
}

func TestCreditCarriesTheLicense(t *testing.T) {
	if c := Load().Credit; !regexp.MustCompile(`MIT License[\s\S]*Jared Andre Rabie`).MatchString(c) {
		t.Fatalf("credit: %q", c)
	}
}
