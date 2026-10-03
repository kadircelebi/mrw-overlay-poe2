package filtereval

import (
	"strconv"
	"strings"
)

// Facts are what the copied item text tells about an item. Zero values mean
// "not known" only where a Known* flag or a pointer says so; the rest are
// facts (an item without sockets has Sockets 0).
type Facts struct {
	Class     string
	BaseType  string
	Rarity    string // "Normal", "Magic", "Rare", "Unique"
	ItemLevel int
	Quality   int
	Sockets   int
	StackSize int
	GemLevel  int
	// WaystoneTier is 0 for anything but a waystone.
	WaystoneTier   int
	Identified     bool
	Corrupted      bool
	TwiceCorrupted bool
	Mirrored       bool
	Fractured      bool
	AnyEnchantment bool
	// ExplicitMods are the explicit modifiers' names as the advanced copy
	// writes them ({ Prefix Modifier "Merciless" ... }).
	ExplicitMods []string
	// AreaLevel is the level of the area the player is in; 0 = unknown.
	AreaLevel int
}

// Outcome of one block or condition against an item.
type Outcome int

const (
	No Outcome = iota
	Yes
	Unknown
)

// Match is a block that matched the item, surely or maybe.
type Match struct {
	Block Block `json:"block"`
	// Unknown lists the conditions the item text could not decide; empty
	// for a certain match.
	Unknown []string `json:"unknown,omitempty"`
}

// Result is the filter's verdict on an item.
type Result struct {
	// Final is the block that decides the item (the first certain match
	// without Continue); nil when no rule matches and the game shows it.
	Final *Match `json:"final,omitempty"`
	// Maybe are blocks above Final that would decide instead if their
	// unknown conditions hold, nearest first.
	Maybe []Match `json:"maybe,omitempty"`
	// Decorations are Continue blocks that matched above Final and lent it
	// their looks (NeverSink's "ilvl 82" or "corrupted" frames).
	Decorations []Match `json:"decorations,omitempty"`
}

// maxMaybe bounds the probable answers listed above the certain one.
const maxMaybe = 3

// Evaluate tries the item against the blocks top to bottom.
func Evaluate(blocks []Block, f Facts) Result {
	var res Result
	for _, b := range blocks {
		outcome, unknown := evalBlock(b, f)
		switch outcome {
		case No:
			continue
		case Unknown:
			if len(res.Maybe) < maxMaybe {
				res.Maybe = append(res.Maybe, Match{Block: b, Unknown: unknown})
			}
			continue
		}
		if b.Continue {
			res.Decorations = append(res.Decorations, Match{Block: b})
			continue
		}
		res.Final = &Match{Block: b}
		return res
	}
	// A Continue block with nothing after it still decides the item.
	if n := len(res.Decorations); n > 0 {
		last := res.Decorations[n-1]
		res.Final = &last
		res.Decorations = res.Decorations[:n-1]
	}
	return res
}

func evalBlock(b Block, f Facts) (Outcome, []string) {
	var unknown []string
	for _, c := range b.Conds {
		switch evalCond(c, f) {
		case No:
			return No, nil
		case Unknown:
			unknown = append(unknown, c.Line)
		}
	}
	if len(unknown) > 0 {
		return Unknown, unknown
	}
	return Yes, nil
}

var rarityRank = map[string]int{"normal": 0, "magic": 1, "rare": 2, "unique": 3}

func evalCond(c Cond, f Facts) Outcome {
	switch c.Key {
	case "Class":
		return boolOutcome(matchStrings(c.Op, c.Values, []string{f.Class}))
	case "BaseType":
		return boolOutcome(matchStrings(c.Op, c.Values, []string{f.BaseType}))
	case "Rarity":
		return evalRarity(c, f.Rarity)
	case "ItemLevel":
		return compareNum(c, f.ItemLevel)
	case "Quality":
		return compareNum(c, f.Quality)
	case "Sockets":
		return compareNum(c, f.Sockets)
	case "StackSize":
		return compareNum(c, max(f.StackSize, 1))
	case "GemLevel":
		return compareNum(c, f.GemLevel)
	case "WaystoneTier":
		return compareNum(c, f.WaystoneTier)
	case "AreaLevel":
		if f.AreaLevel <= 0 {
			return Unknown
		}
		return compareNum(c, f.AreaLevel)
	case "Identified":
		return compareBool(c, f.Identified)
	case "Corrupted":
		return compareBool(c, f.Corrupted)
	case "TwiceCorrupted":
		return compareBool(c, f.TwiceCorrupted)
	case "Mirrored":
		return compareBool(c, f.Mirrored)
	case "Fractured", "FracturedItem":
		return compareBool(c, f.Fractured)
	case "AnyEnchantment":
		return compareBool(c, f.AnyEnchantment)
	case "HasExplicitMod":
		return evalExplicitMods(c, f)
	}
	// UnidentifiedItemTier, Width, Height, BaseArmour, DropLevel... are not
	// in the copied text.
	return Unknown
}

func boolOutcome(ok bool) Outcome {
	if ok {
		return Yes
	}
	return No
}

// matchStrings applies a string list condition: "==" is an exact match,
// otherwise a value matches when it is part of the item's (the game's
// substring rule); "!=" / "!" negate. Case is ignored.
func matchStrings(op string, values, have []string) bool {
	hit := false
	for _, h := range have {
		lh := strings.ToLower(h)
		for _, v := range values {
			lv := strings.ToLower(v)
			if (op == "==" && lh == lv) || (op != "==" && lv != "" && strings.Contains(lh, lv)) {
				hit = true
			}
		}
	}
	if op == "!=" || op == "!" {
		return !hit
	}
	return hit
}

func evalRarity(c Cond, rarity string) Outcome {
	have, ok := rarityRank[strings.ToLower(rarity)]
	if !ok {
		return Unknown
	}
	switch c.Op {
	case "<", "<=", ">", ">=":
		if len(c.Values) == 0 {
			return Unknown
		}
		want, ok := rarityRank[strings.ToLower(c.Values[0])]
		if !ok {
			return Unknown
		}
		return boolOutcome(compare(c.Op, have, want))
	}
	hit := false
	for _, v := range c.Values {
		if r, ok := rarityRank[strings.ToLower(v)]; ok && r == have {
			hit = true
		}
	}
	if c.Op == "!=" || c.Op == "!" {
		hit = !hit
	}
	return boolOutcome(hit)
}

func compareNum(c Cond, have int) Outcome {
	if len(c.Values) == 0 {
		return Unknown
	}
	op := c.Op
	if op == "" {
		op = "="
	}
	// "Sockets 2 3" style lists: any value.
	for _, v := range c.Values {
		want, err := strconv.Atoi(v)
		if err != nil {
			return Unknown
		}
		if compare(op, have, want) {
			return Yes
		}
	}
	return No
}

func compare(op string, a, b int) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "!=", "!":
		return a != b
	}
	return a == b
}

func compareBool(c Cond, have bool) Outcome {
	if len(c.Values) == 0 {
		return Unknown
	}
	want := strings.EqualFold(c.Values[0], "true")
	if c.Op == "!=" || c.Op == "!" {
		want = !want
	}
	return boolOutcome(have == want)
}

// evalExplicitMods counts the item's explicit modifiers whose name matches
// any value and compares the count ("HasExplicitMod >=2 ..."; at least one
// when no count is given). Names are only known on identified items.
func evalExplicitMods(c Cond, f Facts) Outcome {
	if !f.Identified {
		return No // the game cannot see an unidentified item's modifiers
	}
	n := 0
	for _, name := range f.ExplicitMods {
		if matchStrings(c.Op, c.Values, []string{name}) {
			n++
		}
	}
	op, want := c.CountOp, c.Count
	if op == "" && want == 0 {
		op, want = ">=", 1
	} else if op == "" {
		op = "="
	}
	return boolOutcome(compare(op, n, want))
}

// Verdict is the item's fate in a few words.
type Verdict string

const (
	VerdictShow    Verdict = "show"
	VerdictHide    Verdict = "hide"
	VerdictMinimal Verdict = "minimal"
	VerdictNone    Verdict = "none" // no rule matches: the game shows it plainly
)

// VerdictOf is the verdict of a result.
func VerdictOf(r Result) Verdict {
	if r.Final == nil {
		return VerdictNone
	}
	switch r.Final.Block.Action {
	case Hide:
		return VerdictHide
	case Minimal:
		return VerdictMinimal
	}
	return VerdictShow
}
