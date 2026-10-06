package filter

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The Exotic group: NeverSink's exotic gear rules as the base, plus the
// player's differences (entries added, NeverSink entries switched off,
// levels changed). Only the differences are stored, so NeverSink's updates
// keep flowing in while the player's choices stay.

// Exotic levels: two looks, "very valuable" and "valuable".
const (
	ExoticHigh   = "high"
	ExoticNormal = "normal"
)

// Exotic entry kinds.
const (
	ExoticBase = "base" // a valuable base type, any of Conds' rarities
	ExoticMod  = "mod"  // an identified item of Classes with one of Names
)

// ExoticEntry is one line of the Exotic group.
type ExoticEntry struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Base string `json:"base,omitempty"`
	// Classes and Names make a modifier entry: items of these classes with
	// an explicit modifier of one of these names (a stat's tier names).
	Classes []string `json:"classes,omitempty"`
	Names   []string `json:"names,omitempty"`
	// Stat and MinTier record a player's modifier entry (the stat chosen and
	// the lowest tier kept), so the panel can show and edit it.
	Stat    string `json:"stat,omitempty"`
	MinTier int    `json:"min_tier,omitempty"`
	// Label is the modifier as the game prints it, for the panel.
	Label string `json:"label,omitempty"`
	// Conds are the rule's other conditions as written ("Rarity Normal
	// Magic Rare", "ItemLevel >= 82", "Identified True"...).
	Conds  []string `json:"conds,omitempty"`
	Level  string   `json:"level"`
	Source string   `json:"source"` // "neversink" or "user"
	// Tier is NeverSink's tier tag of the rule the entry came from.
	Tier string `json:"tier,omitempty"`
}

// ExoticSettings are the player's differences from NeverSink.
type ExoticSettings struct {
	Added []ExoticEntry `json:"added"`
	// Removed are keys of NeverSink entries switched off.
	Removed []string `json:"removed"`
	// Levels moves entries (NeverSink's or the player's) to another level.
	Levels map[string]string `json:"levels"`
}

// ExoticRow is an entry as the panel lists it.
type ExoticRow struct {
	ExoticEntry
	Off     bool `json:"off"`
	Changed bool `json:"changed"` // level differs from NeverSink's
}

var (
	nsHeaderTierRE  = regexp.MustCompile(`\$tier->(\S+)`)
	nsHeaderStyleRE = regexp.MustCompile(`!(\S+)\s*$`)
)

// styleKeys are the lines that only style a rule.
var styleKeys = []string{"SetFontSize", "SetTextColor", "SetBorderColor", "SetBackgroundColor",
	"PlayAlertSound", "PlayEffect", "MinimapIcon", "CustomAlertSound", "DisableDropSound", "EnableDropSound", "Continue"}

// NeverSinkExotics turns NeverSink's exotic rules (neversink.ExoticBlocks)
// into entries: one per base, one per modifier name.
func NeverSinkExotics(blocks []string) []ExoticEntry {
	var out []ExoticEntry
	seen := map[string]bool{}
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		if len(lines) == 0 {
			continue
		}
		header := lines[0]
		tier := ""
		if m := nsHeaderTierRE.FindStringSubmatch(header); m != nil {
			tier = m[1]
		}
		level := ExoticNormal
		if m := nsHeaderStyleRE.FindStringSubmatch(header); m != nil && strings.HasSuffix(m[1], "btier") {
			level = ExoticHigh
		}
		kind := ExoticBase
		if strings.Contains(header, "$type->exoticmods") {
			kind = ExoticMod
		}
		var conds, bases, classes, names []string
		for _, raw := range lines[1:] {
			line := strings.TrimSpace(raw)
			key, _, _ := strings.Cut(line, " ")
			switch {
			case line == "" || isStyleKey(key):
			case key == "BaseType":
				bases = append(bases, quotedValues(line)...)
			case key == "HasExplicitMod":
				names = append(names, quotedValues(line)...)
			case key == "Class" && kind == ExoticMod:
				classes = append(classes, quotedValues(line)...)
			default:
				conds = append(conds, line)
			}
		}
		add := func(e ExoticEntry) {
			if !seen[e.Key] {
				seen[e.Key] = true
				out = append(out, e)
			}
		}
		if kind == ExoticBase {
			for _, base := range bases {
				add(ExoticEntry{Key: "ns|" + tier + "|" + strings.ToLower(base), Kind: ExoticBase, Base: base,
					Conds: conds, Level: level, Source: "neversink", Tier: tier})
			}
			continue
		}
		for _, name := range names {
			add(ExoticEntry{Key: "ns|" + tier + "|" + strings.ToLower(name), Kind: ExoticMod, Classes: classes,
				Names: []string{name}, Conds: conds, Level: level, Source: "neversink", Tier: tier})
		}
	}
	return out
}

func isStyleKey(key string) bool {
	for _, k := range styleKeys {
		if key == k {
			return true
		}
	}
	return false
}

func quotedValues(line string) []string {
	var out []string
	for _, m := range quotedValueRE.FindAllStringSubmatch(line, -1) {
		out = append(out, m[1])
	}
	return out
}

// UserExoticKey is the key of a player's entry.
func UserExoticKey(e ExoticEntry) string {
	if e.Kind == ExoticBase {
		return "user|base|" + strings.ToLower(e.Base)
	}
	cls := append([]string(nil), e.Classes...)
	sort.Strings(cls)
	return "user|mod|" + strings.ToLower(strings.Join(cls, ",")) + "|" + strings.ToLower(e.Stat+"|"+strings.Join(e.Names, ","))
}

// normalizeExotic tidies the player's differences: entries get their key,
// a default level and their source; duplicates and empty ones go.
func normalizeExotic(x *ExoticSettings) {
	var added []ExoticEntry
	seen := map[string]bool{}
	for _, e := range x.Added {
		e.Source = "user"
		e.Base = CleanText(e.Base)
		e.Stat, e.Label = CleanText(e.Stat), CleanText(e.Label)
		e.Names, e.Classes = cleanTexts(e.Names), cleanTexts(e.Classes)
		// A player's entry is written with fixed conditions (userExoticConds);
		// only NeverSink's own entries carry theirs.
		e.Conds, e.Tier = nil, ""
		if e.Kind != ExoticMod {
			e.Kind = ExoticBase
		}
		if (e.Kind == ExoticBase && e.Base == "") || (e.Kind == ExoticMod && (len(e.Names) == 0 || len(e.Classes) == 0)) {
			continue
		}
		if e.Level != ExoticHigh {
			e.Level = ExoticNormal
		}
		e.Key = UserExoticKey(e)
		if seen[e.Key] {
			continue
		}
		seen[e.Key] = true
		added = append(added, e)
	}
	x.Added = added
	removed := x.Removed[:0:0]
	gone := map[string]bool{}
	for _, k := range x.Removed {
		if k != "" && !gone[k] {
			gone[k] = true
			removed = append(removed, k)
		}
	}
	x.Removed = removed
	for k, v := range x.Levels {
		if v != ExoticHigh && v != ExoticNormal {
			delete(x.Levels, k)
		}
	}
}

// ExoticRows lists NeverSink's entries (switched-off ones marked) followed by
// the player's, with the player's levels applied.
func ExoticRows(ns []ExoticEntry, x ExoticSettings) []ExoticRow {
	off := map[string]bool{}
	for _, k := range x.Removed {
		off[k] = true
	}
	var rows []ExoticRow
	for _, e := range append(append([]ExoticEntry(nil), ns...), x.Added...) {
		r := ExoticRow{ExoticEntry: e, Off: off[e.Key]}
		if lv, ok := x.Levels[e.Key]; ok && lv != e.Level {
			r.Level, r.Changed = lv, e.Source == "neversink"
		}
		rows = append(rows, r)
	}
	return rows
}

// exoticRules writes the Exotic group: entries grouped by level and rule
// shape, NeverSink's conditions kept.
func (b *builder) exoticRules(cfg Config, high, normal *style) {
	for _, level := range []string{ExoticHigh, ExoticNormal} {
		st := normal
		if level == ExoticHigh {
			st = high
		}
		type shape struct{ kind, conds, classes string }
		var order []shape
		lists := map[shape][]string{}
		condsOf := map[shape][]string{}
		for _, r := range ExoticRows(NeverSinkExotics(cfg.Exotics), cfg.Exotic) {
			if r.Off || r.Level != level {
				continue
			}
			conds := r.Conds
			if r.Source == "user" {
				conds = userExoticConds(r.ExoticEntry)
			}
			k := shape{r.Kind, strings.Join(conds, "\n"), strings.Join(r.Classes, "\x00")}
			if _, ok := lists[k]; !ok {
				order = append(order, k)
				condsOf[k] = conds
			}
			if r.Kind == ExoticBase {
				lists[k] = appendNew(lists[k], r.Base)
			} else {
				lists[k] = appendNew(lists[k], r.Names...)
			}
		}
		for _, k := range order {
			conds := append([]string(nil), condsOf[k]...)
			// The $type tag (as NeverSink writes it) lets the filter
			// explanation tell the Exotic group's rules apart.
			if k.kind == ExoticBase {
				b.rule("Show # $type->exoticbases", conds, "BaseType", lists[k], st)
				continue
			}
			if k.classes != "" {
				conds = append(conds, "Class == "+quoteList(strings.Split(k.classes, "\x00")))
			}
			conds = append(conds, "HasExplicitMod >=1 "+quoteList(lists[k]))
			b.rule("Show # $type->exoticmods", conds, "", nil, st)
		}
	}
}

// userExoticConds are a player's entry's conditions: every rarity, and for
// a modifier only identified, unspoiled items (as NeverSink's own rules).
func userExoticConds(e ExoticEntry) []string {
	if e.Kind == ExoticBase {
		return []string{"Rarity Normal Magic Rare"}
	}
	return []string{"Mirrored False", "Corrupted False", "Identified True", "Rarity Normal Magic Rare"}
}

func cleanTexts(in []string) []string {
	var out []string
	for _, s := range in {
		if s = CleanText(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func appendNew(list []string, values ...string) []string {
	for _, v := range values {
		dup := false
		for _, have := range list {
			dup = dup || have == v
		}
		if !dup {
			list = append(list, v)
		}
	}
	return list
}

func quoteList(values []string) string {
	q := make([]string, len(values))
	for i, v := range values {
		q[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(q, " ")
}
