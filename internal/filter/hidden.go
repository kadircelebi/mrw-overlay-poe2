package filter

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"poe2filter/internal/i18n"
	"poe2filter/internal/prices"
)

// HiddenItem is one entry of the "hidden by me" list the player fills with
// Alt+H: a base they do not want to see on the ground, optionally only in
// some rarities or only in small stacks.
type HiddenItem struct {
	Base string `json:"base"`
	// Rarities limits the entry to these ("Normal", "Magic", "Rare",
	// "Unique"); empty means any.
	Rarities []string `json:"rarities,omitempty"`
	// BelowStack hides only stacks smaller than this (0 = any stack).
	BelowStack int `json:"below_stack,omitempty"`
	// WhileCheap keeps the entry behind the valuable-item rules, so an item
	// whose price climbs over the threshold shows again by itself. Without
	// it the entry hides unconditionally.
	WhileCheap bool  `json:"while_cheap"`
	AddedAt    int64 `json:"added_at"`
}

// MaxHiddenItems keeps the list (and the filter) a reasonable size.
const MaxHiddenItems = 500

var hiddenRarities = []string{"Normal", "Magic", "Rare", "Unique"}

// Key identifies an entry: the same base with the same limits is one entry.
func (h HiddenItem) Key() string {
	return strings.ToLower(h.Base) + "|" + strings.Join(h.Rarities, ",") + "|" + fmt.Sprint(h.BelowStack)
}

// normalizeHidden trims and de-duplicates the list (a later entry replaces
// an earlier one with the same key).
func normalizeHidden(in []HiddenItem) []HiddenItem {
	var out []HiddenItem
	index := map[string]int{}
	for _, h := range in {
		h.Base = strings.TrimSpace(h.Base)
		if h.Base == "" {
			continue
		}
		var rar []string
		for _, want := range hiddenRarities {
			for _, r := range h.Rarities {
				if strings.EqualFold(strings.TrimSpace(r), want) {
					rar = append(rar, want)
					break
				}
			}
		}
		if len(rar) == len(hiddenRarities) {
			rar = nil
		}
		h.Rarities = rar
		h.BelowStack = min(max(h.BelowStack, 0), MaxMinStack)
		if i, ok := index[h.Key()]; ok {
			out[i] = h
			continue
		}
		if len(out) == MaxHiddenItems {
			continue
		}
		index[h.Key()] = len(out)
		out = append(out, h)
	}
	return out
}

// HiddenKey is the key an entry has once stored (rarity spelling and order
// normalised), "" for an entry without a base.
func HiddenKey(h HiddenItem) string {
	n := normalizeHidden([]HiddenItem{h})
	if len(n) == 0 {
		return ""
	}
	return n[0].Key()
}

var quotedValueRE = regexp.MustCompile(`"([^"]+)"`)

// exoticBases are the bases the NeverSink exotic rules name; they win over
// the hidden list (the player keeps them by leaving that rule on).
func exoticBases(cfg Config) map[string]bool {
	out := map[string]bool{}
	if !cfg.ShowExotics {
		return out
	}
	for _, block := range cfg.Exotics {
		if !strings.Contains(block, "$type->exoticbases") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "BaseType") {
				for _, m := range quotedValueRE.FindAllStringSubmatch(line, -1) {
					out[strings.ToLower(m[1])] = true
				}
			}
		}
	}
	return out
}

// hiddenRules writes the hidden list's entries whose WhileCheap equals
// whileCheap, one rule per set of limits, and reports whether it wrote any.
// Exotic bases are skipped with a warning; bases the base filter does not
// name are written as given (the game decides).
func (b *builder) hiddenRules(cfg Config, whileCheap bool, canon func(string) (string, bool), snap *prices.Snapshot, thr float64, st *Stats) bool {
	if cfg.HiddenOff {
		return false
	}
	exotic := exoticBases(cfg)
	type limits struct {
		rarities string
		below    int
	}
	groups := map[limits][]string{}
	var order []limits
	for _, h := range cfg.HiddenItems {
		if h.WhileCheap != whileCheap {
			continue
		}
		base := h.Base
		if c, ok := canon(base); ok {
			base = c
		}
		if exotic[strings.ToLower(base)] {
			st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.hiddenExotic"), base))
			continue
		}
		if whileCheap && snap != nil && thr > 0 {
			for _, c := range snap.Currency {
				if strings.EqualFold(c.Name, base) && c.ValueEx >= thr {
					st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.hiddenValuable"), base, c.ValueEx))
				}
			}
		}
		k := limits{strings.Join(h.Rarities, " "), h.BelowStack}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], base)
	}
	for _, k := range order {
		var conds []string
		if k.rarities != "" {
			conds = append(conds, "Rarity "+k.rarities)
		}
		if k.below > 0 {
			conds = append(conds, fmt.Sprintf("StackSize < %d", k.below))
		}
		bases := groups[k]
		sort.Strings(bases)
		b.rule("Hide", conds, "BaseType", bases, nil)
	}
	return len(order) > 0
}
