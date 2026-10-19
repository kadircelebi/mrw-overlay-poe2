package invscan

import (
	"regexp"
	"strconv"
	"strings"
)

var levelName = regexp.MustCompile(`\(Level (\d+)\)$`)

// Pick chooses which of the items sharing an icon a cell holds, by the
// number the game writes bottom right: a level for items named "… (Level
// N)" (uncut gems, Thaumaturgic Flux), II/III for an orb's Greater and
// Perfect tiers, nothing for the plain one. ok is false when the marks do
// not tell (an icon shared by unrelated items, a level not listed, an
// unsure reading).
func Pick(entries []Entry, m Marks) (Entry, bool) {
	if len(entries) == 0 {
		return Entry{}, false
	}
	if len(entries) == 1 {
		return entries[0], true
	}
	if m.Unsure {
		return Entry{}, false
	}
	levelled := false
	for _, e := range entries {
		if g := levelName.FindStringSubmatch(e.Name); g != nil {
			levelled = true
			if n, _ := strconv.Atoi(g[1]); n == m.Corner {
				return e, true
			}
		}
	}
	if levelled {
		return Entry{}, false
	}
	// Tiers: the plain orb, "Greater …" and "Perfect …".
	want := ""
	switch {
	case m.Corner == 0:
	case m.Roman && m.Corner == 2:
		want = "Greater "
	case m.Roman && m.Corner == 3:
		want = "Perfect "
	default:
		return Entry{}, false
	}
	var plain *Entry
	for i, e := range entries {
		if want != "" && strings.HasPrefix(e.Name, want) {
			return e, true
		}
		if !strings.HasPrefix(e.Name, "Greater ") && !strings.HasPrefix(e.Name, "Perfect ") {
			if plain != nil {
				return Entry{}, false // two unrelated items share the icon
			}
			plain = &entries[i]
		}
	}
	if want == "" && plain != nil {
		return *plain, true
	}
	return Entry{}, false
}
