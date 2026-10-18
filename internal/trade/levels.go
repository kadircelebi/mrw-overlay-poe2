package trade

import (
	_ "embed"
	"encoding/json"
	"sort"
	"sync"
)

// MinScanLevel is the lowest required level of a base the exceptional scan
// prices. Exceptional items that are worth something are endgame bases; the
// low ones took most of the searches and next to none of the value.
const MinScanLevel = 65

// base_levels.json is written by build/craft/build_data.py next to the craft
// data, from the same RePoE base list.
//
//go:embed base_levels.json
var baseLevelsJSON []byte

var baseLevels = sync.OnceValue(func() map[string]int {
	var f struct {
		Levels map[string]int `json:"levels"`
	}
	if err := json.Unmarshal(baseLevelsJSON, &f); err != nil {
		panic("trade: base_levels.json: " + err.Error())
	}
	return f.Levels
})

// BaseLevel returns the level a base requires; ok is false for a base the
// table does not know (one that does not drop, or one newer than the data).
func BaseLevel(base string) (level int, ok bool) {
	level, ok = baseLevels()[base]
	return level, ok
}

// Exceptions are the low-level bases scanned and shown anyway: NeverSink's
// valuable exceptional bases (a caster weapon's implicit can carry it), its
// Orb of Chance targets, and the low bases known to sell as exceptional
// crafting bases (lowValuable).
type Exceptions struct {
	Valuable map[string]bool
	Chance   map[string]bool
}

// lowValuable are low-level bases whose exceptional copies sell although
// NeverSink lists them as neither: a Jade Tiara with two rune sockets
// trades for a few Chaos Orbs.
var lowValuable = map[string]bool{
	"Jade Tiara": true,
}

func (e Exceptions) has(base string) bool {
	return e.Valuable[base] || e.Chance[base] || lowValuable[base]
}

// BuildCandidates keeps the bases worth scanning: those required at
// MinScanLevel or above, plus the exceptions. A base of unknown level is
// left out. NeverSink's valuable bases are scanned first.
func BuildCandidates(bases []string, ex Exceptions) []Candidate {
	var out []Candidate
	for _, b := range bases {
		level, ok := BaseLevel(b)
		if !ex.has(b) && (!ok || level < MinScanLevel) {
			continue
		}
		p := 1
		if ex.Valuable[b] {
			p = 0
		}
		out = append(out, Candidate{Base: b, Priority: p})
	}
	return out
}

// LowLevelBases returns, sorted, the bases below MinScanLevel that are not
// exceptions: the ones whose exceptional drops the filter hides unless the
// player asks for them.
func LowLevelBases(bases []string, ex Exceptions) []string {
	var out []string
	for _, b := range bases {
		if level, ok := BaseLevel(b); ok && level < MinScanLevel && !ex.has(b) {
			out = append(out, b)
		}
	}
	sort.Strings(out)
	return out
}
