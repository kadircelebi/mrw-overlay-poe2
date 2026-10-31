// Package campaign holds the campaign's areas, by the ids the game logs for
// them, with what to watch for in each.
package campaign

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// The kinds of note, as the box and the tracker mark them.
const (
	KindMust     = "must"     // a lasting reward or a step not to miss
	KindOptional = "optional" // worth it, but can be skipped
	KindTip      = "tip"      // how to find the way
	KindNext     = "next"     // where to go on
)

// Note is one thing to watch for in an area, in English and Turkish.
type Note struct {
	Kind string `json:"kind"`
	EN   string `json:"en"`
	TR   string `json:"tr"`
}

// Area is one campaign area. ID is what the game logs for it, Also the other
// ids the same area can be built under; Name is its name in the game.
type Area struct {
	ID    string   `json:"id"`
	Also  []string `json:"also,omitempty"`
	Name  string   `json:"name"`
	Notes []Note   `json:"notes"`
}

// Act is an act or interlude, its areas in the order they are played.
type Act struct {
	ID    string `json:"id"`
	EN    string `json:"en"`
	TR    string `json:"tr"`
	Areas []Area `json:"areas"`
}

// Guide is the whole campaign. Credit is the notice of the notes' source.
type Guide struct {
	Acts   []Act  `json:"acts"`
	Credit string `json:"credit"`
}

//go:embed guide.json
var guideJSON []byte

//go:embed LICENSE.path-of-levelling-2
var credit string

// Load returns the guide; callers must not change it.
var Load = sync.OnceValue(func() *Guide {
	g := &Guide{}
	if err := json.Unmarshal(guideJSON, g); err != nil {
		panic("campaign: guide.json: " + err.Error())
	}
	g.Credit = credit
	return g
})

// Find returns the area the game logs as id, with its act.
func Find(id string) (Area, Act, bool) {
	for _, act := range Load().Acts {
		for _, a := range act.Areas {
			if a.ID == id {
				return a, act, true
			}
			for _, other := range a.Also {
				if other == id {
					return a, act, true
				}
			}
		}
	}
	return Area{}, Act{}, false
}
