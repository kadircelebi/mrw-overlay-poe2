package overlay

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Pseudo totals: the trade site adds up an item's lines into "pseudo" stats
// (total Life, total Fire Resistance, total Attributes…) and can search by
// them. Pricing a rare by its totals finds items that reach the same numbers
// by other rolls, as POE2 Overlay does.

// pseudoPart is one total a source line feeds.
type pseudoPart string

const (
	pLife      pseudoPart = "life"
	pMana      pseudoPart = "mana"
	pStr       pseudoPart = "str"
	pDex       pseudoPart = "dex"
	pInt       pseudoPart = "int"
	pFire      pseudoPart = "fire"
	pCold      pseudoPart = "cold"
	pLightning pseudoPart = "lightning"
	pChaos     pseudoPart = "chaos"
	pMove      pseudoPart = "move"
)

// pseudoSources maps the wording of a modifier (as normalizeStat reads it) to
// the totals it adds to. Lines of every kind count: implicit, explicit,
// crafted, fractured, desecrated, rune and enchant.
var pseudoSources = func() map[string][]pseudoPart {
	src := map[string][]pseudoPart{
		"# to maximum Life":                     {pLife},
		"# to maximum Mana":                     {pMana},
		"# to Strength":                         {pStr},
		"# to Dexterity":                        {pDex},
		"# to Intelligence":                     {pInt},
		"# to Strength and Dexterity":           {pStr, pDex},
		"# to Strength and Intelligence":        {pStr, pInt},
		"# to Dexterity and Intelligence":       {pDex, pInt},
		"# to all Attributes":                   {pStr, pDex, pInt},
		"#% to Fire Resistance":                 {pFire},
		"#% to Cold Resistance":                 {pCold},
		"#% to Lightning Resistance":            {pLightning},
		"#% to Chaos Resistance":                {pChaos},
		"#% to all Elemental Resistances":       {pFire, pCold, pLightning},
		"#% to Fire and Cold Resistances":       {pFire, pCold},
		"#% to Fire and Lightning Resistances":  {pFire, pLightning},
		"#% to Cold and Lightning Resistances":  {pCold, pLightning},
		"#% to Fire and Chaos Resistances":      {pFire, pChaos},
		"#% to Cold and Chaos Resistances":      {pCold, pChaos},
		"#% to Lightning and Chaos Resistances": {pLightning, pChaos},
		"#% increased Movement Speed":           {pMove},
	}
	out := make(map[string][]pseudoPart, len(src))
	for text, parts := range src {
		out[normalizeStat(text)] = parts
	}
	return out
}()

// pseudoStat is one pseudo line offered for the search.
type pseudoStat struct {
	id, text string
	// covers are the parts whose source lines this total replaces in the
	// search; a line all of whose parts are covered starts unselected.
	covers []pseudoPart
	// hidden totals start unselected and folded away: another total already
	// says the same (Fire beside total Elemental Resistance).
	hidden bool
}

// addPseudoTotals appends the item's pseudo totals. For a magic or rare item
// they start selected and the lines they add up start unselected, so the
// search asks for the totals rather than the exact rolls. A unique is priced
// by its own modifiers, except Headhunter's combined Attributes.
func addPseudoTotals(item *Item, opts ParseOptions) {
	addTotals(item)
	addKindSums(item, opts)
	addElementalAttackSum(item, opts)
}

// kindSums are stats worth searching across every kind they come in. The
// trade site already adds a prefix and a suffix of the same kind into one
// line, but a fractured, rune or enchant roll is a stat of its own; a
// Weighted Sum v2 group of all of them finds items that reach the same
// total either way (18% explicit + 18% fractured Rarity is 36).
var kindSums = []struct {
	stat, text string
	kinds      []string
}{
	{"stat_3917489142", "#% increased Rarity of Items found", []string{"explicit", "implicit", "fractured", "crafted", "enchant", "rune", "desecrated"}},
}

func addKindSums(item *Item, opts ParseOptions) {
	selected := opts.SignedIn && (item.Rarity == "magic" || item.Rarity == "rare")
	for _, sum := range kindSums {
		var sources []int
		var total float64
		for i, mod := range item.Mods {
			if mod.Type == "pseudo" || len(mod.Values) == 0 || !strings.HasSuffix(mod.StatID, "."+sum.stat) {
				continue
			}
			total += mod.Values[0]
			sources = append(sources, i)
		}
		if len(sources) == 0 || total == 0 {
			continue
		}
		stats := make([]string, 0, len(sum.kinds))
		for _, kind := range sum.kinds {
			stats = append(stats, kind+"."+sum.stat)
		}
		item.Mods = append(item.Mods, ItemMod{
			Key: "mod-" + strconv.Itoa(len(item.Mods)+1), StatID: "sum." + sum.stat, Type: "pseudo",
			Text:   "Sum " + formatPseudo(total) + ": " + sum.text,
			Values: []float64{total}, Selected: selected, WeightStats: stats,
		})
		item.Mods[len(item.Mods)-1].Hidden = !selected
		if selected {
			for _, i := range sources {
				item.Mods[i].Selected, item.Mods[i].Hidden = false, true
			}
		}
	}
}

func addTotals(item *Item) {
	sum := map[pseudoPart]float64{}
	seen := map[pseudoPart]bool{}
	// sources are the mods that feed a total, with the parts of each line; a
	// hybrid mod ("% increased Energy Shield" + "to maximum Life") is one mod
	// of several lines, and only some of them may feed a total.
	sources := map[int][][]pseudoPart{}
	for i, mod := range item.Mods {
		if mod.Type == "pseudo" || mod.Type == "skill" {
			continue
		}
		for _, line := range strings.Split(mod.Text, "\n") {
			parts := pseudoSources[normalizeStat(line)]
			values := valuesOf(line)
			if len(values) == 0 {
				parts = nil
			}
			for _, part := range parts {
				sum[part] += values[0]
				seen[part] = true
			}
			sources[i] = append(sources[i], parts)
		}
	}
	if len(seen) == 0 {
		return
	}

	var stats []pseudoStat
	add := func(id, text string, value float64, covers ...pseudoPart) {
		if value == 0 {
			return
		}
		if value < 0 {
			text = strings.Replace(text, "+#", "#", 1)
		}
		stats = append(stats, pseudoStat{id: id, text: strings.Replace(text, "#", formatPseudo(value), 1), covers: covers})
	}
	// GGG counts two Life per Strength in the total (measured on the trade
	// site: +61 Life and +20 Strength meet "total maximum Life" 101), and
	// likewise two Mana per Intelligence.
	if seen[pLife] || seen[pStr] {
		add("pseudo.pseudo_total_life", "+# total maximum Life", sum[pLife]+2*sum[pStr], pLife)
	}
	if seen[pMana] || seen[pInt] {
		add("pseudo.pseudo_total_mana", "+# total maximum Mana", sum[pMana]+2*sum[pInt], pMana)
	}
	attributes := 0
	for _, a := range []struct {
		part     pseudoPart
		id, text string
	}{
		{pStr, "pseudo.pseudo_total_strength", "+# total to Strength"},
		{pDex, "pseudo.pseudo_total_dexterity", "+# total to Dexterity"},
		{pInt, "pseudo.pseudo_total_intelligence", "+# total to Intelligence"},
	} {
		if seen[a.part] {
			attributes++
			add(a.id, a.text, sum[a.part], a.part)
		}
	}
	if attributes > 1 {
		add("pseudo.pseudo_total_attributes", "+# total to Attributes", sum[pStr]+sum[pDex]+sum[pInt], pStr, pDex, pInt)
	}
	elements := 0
	firstElement := len(stats)
	for _, r := range []struct {
		part     pseudoPart
		id, text string
	}{
		{pFire, "pseudo.pseudo_total_fire_resistance", "+#% total to Fire Resistance"},
		{pCold, "pseudo.pseudo_total_cold_resistance", "+#% total to Cold Resistance"},
		{pLightning, "pseudo.pseudo_total_lightning_resistance", "+#% total to Lightning Resistance"},
	} {
		if seen[r.part] {
			elements++
			add(r.id, r.text, sum[r.part], r.part)
		}
	}
	if seen[pChaos] {
		add("pseudo.pseudo_total_chaos_resistance", "+#% total to Chaos Resistance", sum[pChaos], pChaos)
	}
	elemental := sum[pFire] + sum[pCold] + sum[pLightning]
	if elements > 1 {
		// The elemental total stands in for each element's total and for the
		// lines behind them; those stay on offer, folded away.
		for i := firstElement; i < firstElement+elements && i < len(stats); i++ {
			stats[i].hidden = true
		}
		add("pseudo.pseudo_total_elemental_resistance", "+#% total Elemental Resistance", elemental, pFire, pCold, pLightning)
	}
	if elements > 0 && seen[pChaos] {
		add("pseudo.pseudo_total_resistance", "+#% total Resistance", elemental+sum[pChaos])
		stats[len(stats)-1].hidden = true
	}
	if seen[pMove] {
		add("pseudo.pseudo_increased_movement_speed", "#% increased Movement Speed", sum[pMove], pMove)
	}
	if len(stats) == 0 {
		return
	}

	// Headhunter is priced by combined Attributes, including corruption
	// enchants, rather than separate Strength and Dexterity rolls.
	headHunter := item.Rarity == "unique" && item.Name == "Headhunter"
	priced := item.Rarity == "magic" || item.Rarity == "rare"
	covered := map[pseudoPart]bool{}
	for _, stat := range stats {
		value := valuesOf(stat.text)
		selected := (priced && !stat.hidden) || (headHunter && stat.id == "pseudo.pseudo_total_attributes")
		item.Mods = append(item.Mods, ItemMod{
			Key: "mod-" + strconv.Itoa(len(item.Mods)+1), StatID: stat.id, Type: "pseudo",
			Text: stat.text, Values: value, Selected: selected, Hidden: !selected,
		})
		if !selected {
			continue
		}
		for _, part := range stat.covers {
			covered[part] = true
		}
	}
	if len(covered) == 0 {
		return
	}
	// A mod leaves the search when a total stands in for every line of it; a
	// hybrid with a line no total covers stays as it was.
	for i, lines := range sources {
		all := true
		for _, parts := range lines {
			if len(parts) == 0 {
				all = false
			}
			for _, part := range parts {
				all = all && covered[part]
			}
		}
		if all {
			item.Mods[i].Selected, item.Mods[i].Hidden = false, true
		}
	}
}

func formatPseudo(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// attackDamageStats are the trade stats of "Adds # to # <element> damage to
// Attacks" in every kind the catalog has them.
var attackDamageStats = map[string][]string{
	"fire":      {"explicit.stat_1573130764", "implicit.stat_1573130764", "fractured.stat_1573130764", "desecrated.stat_1573130764", "rune.stat_1573130764"},
	"cold":      {"explicit.stat_4067062424", "fractured.stat_4067062424", "desecrated.stat_4067062424"},
	"lightning": {"explicit.stat_1754445556", "implicit.stat_1754445556", "fractured.stat_1754445556", "desecrated.stat_1754445556"},
}

var attackDamageRE = regexp.MustCompile(`(?i)^Adds \S+ to \S+ (?:Fire|Cold|Lightning) damage to Attacks$`)

// elementalAttackSumID keys the sum line; it is not a trade stat, the line
// is searched through its WeightStats.
const elementalAttackSumID = "sum.elemental_attack_damage"

// addElementalAttackSum adds up the average hits of the item's "Adds # to #
// <element> damage to Attacks" lines, as the trade site does for a Weighted
// Sum v2 group of those stats (2 to 42 counts 22). POE2 Overlay searches
// gloves and quivers this way, so a Cold roll can stand in for a Lightning
// one. Offered only when two or more lines feed it.
func addElementalAttackSum(item *Item, opts ParseOptions) {
	var sources []int
	var total float64
	for i, mod := range item.Mods {
		if mod.Type == "pseudo" || strings.Contains(mod.Text, "\n") {
			continue
		}
		if !attackDamageRE.MatchString(rangeRE.ReplaceAllString(mod.Text, "")) || len(mod.Values) != 2 {
			continue
		}
		total += (mod.Values[0] + mod.Values[1]) / 2
		sources = append(sources, i)
	}
	if len(sources) < 2 {
		return
	}
	var stats []string
	for _, element := range []string{"fire", "cold", "lightning"} {
		stats = append(stats, attackDamageStats[element]...)
	}
	selected := opts.SignedIn && (item.Rarity == "magic" || item.Rarity == "rare")
	value := math.Round(total*100) / 100
	item.Mods = append(item.Mods, ItemMod{
		Key: "mod-" + strconv.Itoa(len(item.Mods)+1), StatID: elementalAttackSumID, Type: "pseudo",
		Text:   "Sum " + formatPseudo(value) + ": Adds # to # Elemental Damage to Attacks",
		Values: []float64{value}, Selected: selected, WeightStats: stats, Hidden: !selected,
	})
	if selected {
		for _, i := range sources {
			item.Mods[i].Selected, item.Mods[i].Hidden = false, true
		}
	}
}
