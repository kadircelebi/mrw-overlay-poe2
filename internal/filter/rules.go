package filter

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
)

const defaultChunkSize = 15

// Minimum evidence before we are willing to HIDE something. Showing a junk
// item costs a glance; hiding a valuable one costs the item.
const (
	minUniqueListingsToHide      = 3
	minExceptionalListingsToHide = 10
	minExceptionalSamplesToHide  = 3
)

// Stats summarises what the generated block does.
type Stats struct {
	ThresholdEx      float64
	ValuableCurrency int
	CheapCurrency    int
	ValuableUniques  int
	CheapUniques     int
	ValuableExcept   int
	CheapExcept      int
	UnknownExceptOn  bool
	Warnings         []string
}

type style struct {
	font             int
	text, border, bg string
	beam, icon       string
	sound            string
}

type builder struct {
	lines []string
}

func (b *builder) add(l ...string) { b.lines = append(b.lines, l...) }

func (b *builder) section(title string) {
	b.add("#==============================================================================",
		"# "+title,
		"#==============================================================================")
}

// rule writes one Show/Hide block. Values of list conditions are chunked so
// no single line becomes unreasonably long.
func (b *builder) rule(action string, conds []string, listKey string, list []string, st *style) {
	emit := func(extra string) {
		b.add(action)
		for _, c := range conds {
			b.add("    " + c)
		}
		if extra != "" {
			b.add("    " + extra)
		}
		if st != nil {
			if st.font > 0 {
				b.add(fmt.Sprintf("    SetFontSize %d", st.font))
			}
			if st.text != "" {
				b.add("    SetTextColor " + st.text)
			}
			if st.border != "" {
				b.add("    SetBorderColor " + st.border)
			}
			if st.bg != "" {
				b.add("    SetBackgroundColor " + st.bg)
			}
			if st.beam != "" {
				b.add("    PlayEffect " + st.beam)
			}
			if st.icon != "" {
				b.add("    MinimapIcon " + st.icon)
			}
			if st.sound != "" {
				b.add("    PlayAlertSound " + st.sound)
			}
		}
		b.add("")
	}
	if listKey == "" {
		emit("")
		return
	}
	for _, ch := range chunkSlice(uniqueStrings(list), defaultChunkSize) {
		emit(fmt.Sprintf("%s == %s", listKey, strings.Join(quoteItems(ch), " ")))
	}
}

var (
	styleMax = &style{font: 45, text: "255 255 255 255", border: "255 215 0 255", bg: "180 0 0 255",
		beam: "Red", icon: "0 Red Star", sound: "6 300"}
	styleUnique = &style{font: 44, text: "255 255 255 255", border: "255 100 0 255", bg: "175 40 0 255",
		beam: "Red", icon: "0 Red Star", sound: "6 300"}
	styleChance = &style{font: 38, text: "0 240 255 255", border: "0 200 255 255", bg: "10 30 50 240",
		icon: "2 Cyan Circle"}
	styleExceptional = &style{font: 42, text: "255 255 255 255", border: "0 210 255 255", bg: "0 40 70 240",
		beam: "Cyan", icon: "1 Cyan Diamond", sound: "2 300"}
	styleExceptionalUnknown = &style{font: 36, text: "200 230 255 255", border: "0 150 200 255", bg: "0 25 45 220"}
	styleT5Rare             = &style{font: 40, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
		icon: "2 Yellow Diamond"}
	styleDim = &style{font: 18, text: "120 120 120 180", border: "0 0 0 0", bg: "0 0 0 150"}
)

// gearClasses are equipment classes (jewels and flasks excluded).
var gearClasses = []string{
	"Amulets", "Belts", "Body Armours", "Boots", "Bows", "Bucklers", "Crossbows",
	"Foci", "Gloves", "Helmets", "One Hand Maces", "Quarterstaves", "Quivers", "Rings",
	"Sceptres", "Shields", "Spears", "Staves", "Talismans", "Two Hand Maces", "Wands",
}

// GenerateDynamicFilterBlock builds the rules injected ahead of the base filter.
// The PoE filter language stops at the first matching block, so ORDER MATTERS:
// explicit user intent first, then valuable drops, then hides, and the blanket
// equipment hide last.
func GenerateDynamicFilterBlock(cfg Config, snap *prices.Snapshot, validBases map[string]string) (string, Stats) {
	st := Stats{ThresholdEx: cfg.ThresholdEx(snap.Rates)}
	thr := st.ThresholdEx
	divEx := snap.Rates.DivineEx
	canon := func(name string) (string, bool) {
		c, ok := validBases[strings.ToLower(strings.TrimSpace(name))]
		return c, ok
	}
	b := &builder{}

	// ---- classify prices ------------------------------------------------
	type cur struct {
		name, cat string
		ex        float64
	}
	var valuableCur, cheapCur []cur
	for _, c := range snap.Currency {
		name, ok := canon(c.Name)
		if !ok || name == "Divine Orb" {
			continue
		}
		if c.ValueEx >= thr {
			valuableCur = append(valuableCur, cur{name, c.Category, c.ValueEx})
		} else {
			cheapCur = append(cheapCur, cur{name, c.Category, c.ValueEx})
		}
	}

	uniqueToBase := map[string]string{}
	var valuableUniqueBases, cheapUniqueBases []string
	for base, ub := range snap.UniqueBases {
		name, ok := canon(base)
		if !ok {
			continue
		}
		for _, u := range ub.Uniques {
			uniqueToBase[strings.ToLower(u.Name)] = name
		}
		if ub.MaxEx >= thr {
			valuableUniqueBases = append(valuableUniqueBases, name)
			continue
		}
		// Hide only when every unique on the base has enough listings to trust.
		trusted := true
		for _, u := range ub.Uniques {
			if u.Listings < minUniqueListingsToHide {
				trusted = false
			}
		}
		if trusted {
			cheapUniqueBases = append(cheapUniqueBases, name)
		}
	}
	sort.Strings(valuableUniqueBases)
	sort.Strings(cheapUniqueBases)
	st.ValuableUniques, st.CheapUniques = len(valuableUniqueBases), len(cheapUniqueBases)

	type exGroup struct {
		kind prices.ExceptionalKind
		min  int
	}
	valuableEx := map[exGroup][]string{}
	cheapEx := map[exGroup][]string{}
	for _, e := range snap.Exceptional {
		name, ok := canon(e.Base)
		if !ok {
			continue
		}
		g := exGroup{e.Kind, e.Min}
		switch {
		case e.Samples > 0 && e.ValueEx >= thr:
			valuableEx[g] = append(valuableEx[g], name)
			st.ValuableExcept++
		case e.Listings >= minExceptionalListingsToHide && e.Samples >= minExceptionalSamplesToHide:
			cheapEx[g] = append(cheapEx[g], name)
			st.CheapExcept++
		}
	}

	// ---- header -----------------------------------------------------------
	b.add("#==============================================================================",
		"# [[DYNAMIC LOOT FILTER]] - poe2-filter (poe.ninja, poe2scout, trade exceptional scan)",
		fmt.Sprintf("# Threshold: %.2f %s (= %.1f Exalted)", cfg.MinValue, cfg.MinValueUnit, thr),
		fmt.Sprintf("# Exchange Rate: 1 Divine = %.1f Exalted", divEx),
		fmt.Sprintf("# Prices generated: %s", snap.GeneratedAt.Local().Format("2006-01-02 15:04")),
		fmt.Sprintf("# Filter written:   %s", time.Now().Format("2006-01-02 15:04")),
		fmt.Sprintf("# Low-value action: %s", strings.ToUpper(cfg.FilterMode)),
		"#==============================================================================",
		"")

	// ---- 1. user blacklist (first, so it really is unconditional) ---------
	if len(cfg.Blacklist) > 0 {
		var uniqueBases, bases []string
		for _, raw := range cfg.Blacklist {
			key := strings.ToLower(strings.TrimSpace(raw))
			if base, ok := uniqueToBase[key]; ok {
				// Never hide a base because of one junk unique if another
				// unique on the same base is valuable.
				if ub := snap.UniqueBases[base]; ub.MaxEx >= thr && !strings.EqualFold(ub.TopName, raw) {
					st.Warnings = append(st.Warnings, fmt.Sprintf(
						"Kara liste: %q atlandı, aynı taban (%s) üzerinde değerli %s var", raw, base, ub.TopName))
					continue
				}
				uniqueBases = append(uniqueBases, base)
			} else if name, ok := canon(raw); ok {
				bases = append(bases, name)
			} else {
				st.Warnings = append(st.Warnings, fmt.Sprintf("Kara liste: %q tanınmadı", raw))
			}
		}
		if len(uniqueBases)+len(bases) > 0 {
			b.section("1. USER BLACKLIST (always hidden)")
			b.rule("Hide", []string{"Rarity == Unique"}, "BaseType", uniqueBases, nil)
			// Rarity <= Rare: blacklisting a base must not hide uniques on it.
			b.rule("Hide", []string{"Rarity <= Rare"}, "BaseType", bases, nil)
		}
	}

	// ---- 2. divine spotlight ----------------------------------------------
	theme, ok := DivineThemes[cfg.DivineTheme]
	if !ok {
		theme = DivineThemes["neon_cyan"]
	}
	b.section("2. DIVINE ORB SPOTLIGHT")
	b.add("Show", `    Class == "Stackable Currency"`, `    BaseType == "Divine Orb"`, "    SetFontSize 45",
		"    SetTextColor "+theme.TextColor, "    SetBorderColor "+theme.Border,
		"    SetBackgroundColor "+theme.BgColor, "    PlayEffect "+theme.Beam,
		"    MinimapIcon 0 "+theme.IconColor+" Star", "    PlayAlertSound 6 300")
	switch s := cfg.DivineSound; {
	case s == "nebu" || s == "nebu.mp3":
		b.add(`    CustomAlertSound "nebu.mp3" 300`)
	case s != "" && s != "auto" && s != "1" && s != "6":
		b.add(fmt.Sprintf(`    CustomAlertSound "%s" 300`, strings.ReplaceAll(s, `"`, "")))
	}
	b.add("")

	if cfg.HideExalt {
		b.rule("Hide", []string{`Class == "Stackable Currency"`, `BaseType == "Exalted Orb"`}, "", nil, nil)
	}
	if cfg.HideGold {
		b.rule("Hide", []string{`Class == "Stackable Currency"`, `BaseType == "Gold"`}, "", nil, nil)
	}

	// ---- 3. user whitelist ------------------------------------------------
	if len(cfg.Whitelist) > 0 {
		var uniqueBases, bases []string
		for _, raw := range cfg.Whitelist {
			key := strings.ToLower(strings.TrimSpace(raw))
			if base, ok := uniqueToBase[key]; ok {
				uniqueBases = append(uniqueBases, base)
			} else if name, ok := canon(raw); ok {
				bases = append(bases, name)
			}
		}
		if len(uniqueBases)+len(bases) > 0 {
			b.section("3. USER WHITELIST (always shown)")
			b.rule("Show", []string{"Rarity == Unique"}, "BaseType", uniqueBases, styleMax)
			b.rule("Show", nil, "BaseType", bases, styleMax)
		}
	}

	// ---- 4. valuable currency and bulk items -------------------------------
	if len(valuableCur) > 0 {
		b.section("4. VALUABLE CURRENCY & BULK ITEMS")
		byCat := map[string][]cur{}
		for _, c := range valuableCur {
			byCat[c.cat] = append(byCat[c.cat], c)
		}
		cats := make([]string, 0, len(byCat))
		for c := range byCat {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, cat := range cats {
			th, ok := CategoryThemes[cat]
			if !ok {
				th = CategoryThemes["currency"]
			}
			var apex, high []string
			for _, c := range byCat[cat] {
				if c.ex >= divEx {
					apex = append(apex, c.name)
				} else {
					high = append(high, c.name)
				}
			}
			sort.Strings(apex)
			sort.Strings(high)
			b.add("# --- " + th.Name + " ---")
			b.rule("Show", nil, "BaseType", apex, &style{font: 45, text: "255 255 255 255",
				border: "255 215 0 255", bg: th.BgT1, beam: th.Beam, icon: "0 " + th.IconColor + " Star", sound: "6 300"})
			b.rule("Show", nil, "BaseType", high, &style{font: 42, text: th.Text, border: th.Border,
				bg: th.BgT1, beam: th.Beam, icon: "1 " + th.IconColor + " " + th.IconShape, sound: "1 300"})
		}
		st.ValuableCurrency = len(valuableCur)
	}

	// ---- 5. valuable unique bases ------------------------------------------
	if len(valuableUniqueBases) > 0 {
		b.section("5. VALUABLE UNIQUE BASES (best unique on the base >= threshold)")
		b.rule("Show", []string{"Rarity == Unique"}, "BaseType", valuableUniqueBases, styleUnique)
	}

	// ---- 6. chance bases ---------------------------------------------------
	if len(cfg.ChanceBases) > 0 {
		var bases []string
		for _, raw := range cfg.ChanceBases {
			if base, ok := uniqueToBase[strings.ToLower(strings.TrimSpace(raw))]; ok {
				bases = append(bases, base)
			} else if name, ok := canon(raw); ok {
				bases = append(bases, name)
			}
		}
		if len(bases) > 0 {
			b.section("6. CHANCE & CRAFTING BASES")
			b.rule("Show", []string{"Rarity <= Magic"}, "BaseType", bases, styleChance)
		}
	}

	// ---- 7. valuable exceptional bases (trade scan) -------------------------
	exGroups := func(m map[exGroup][]string) []exGroup {
		var gs []exGroup
		for g := range m {
			gs = append(gs, g)
		}
		sort.Slice(gs, func(i, j int) bool {
			if gs[i].kind != gs[j].kind {
				return gs[i].kind < gs[j].kind
			}
			return gs[i].min < gs[j].min
		})
		return gs
	}
	exCond := func(g exGroup) string {
		if g.kind == prices.KindQuality {
			return fmt.Sprintf("Quality >= %d", g.min)
		}
		return fmt.Sprintf("Sockets >= %d", g.min)
	}
	if len(valuableEx) > 0 {
		b.section("7. VALUABLE EXCEPTIONAL BASES (trade scan)")
		for _, g := range exGroups(valuableEx) {
			sort.Strings(valuableEx[g])
			b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic Rare", exCond(g)},
				"BaseType", valuableEx[g], styleExceptional)
		}
	}

	// ---- 8. rares, jewels, quality, waystones, gems, keys --------------------
	if cfg.T5Rares {
		b.section("8.1 TIER 5 RARE EQUIPMENT")
		b.rule("Show", []string{"Rarity == Rare", "UnidentifiedItemTier >= 5"}, "Class", gearClasses, styleT5Rare)
	}
	if cfg.IncludeGear {
		b.section("8.2 RARE JEWELS")
		if cfg.T5JewelsOnly {
			b.rule("Show", []string{`Class == "Jewels"`, "Rarity == Rare", "UnidentifiedItemTier >= 5"}, "", nil,
				&style{font: 42, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
					beam: "Yellow", icon: "1 Yellow Diamond", sound: "2 300"})
		} else {
			b.rule("Show", []string{`Class == "Jewels"`, "Rarity == Rare"}, "", nil,
				&style{font: 40, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255", icon: "2 Yellow Diamond"})
		}
		// Rarity <= Rare: unique jewels follow the unique rules above.
		b.rule("Hide", []string{`Class == "Jewels"`, "Rarity <= Rare"}, "", nil, nil)
	}
	if cfg.QualityThreshold > 0 {
		b.section(fmt.Sprintf("8.3 HIGH QUALITY GEAR (Quality >= %d)", cfg.QualityThreshold))
		b.rule("Show", []string{"Rarity <= Rare", fmt.Sprintf("Quality >= %d", cfg.QualityThreshold)}, "Class", gearClasses,
			&style{font: 40, text: "255 255 255 255", border: "255 215 0 255", bg: "40 30 0 240", icon: "1 Yellow Diamond"})
	}
	if cfg.HighWaystones {
		b.section("8.4 HIGH-TIER WAYSTONES (T14+)")
		b.rule("Show", []string{`Class == "Waystones"`, "WaystoneTier >= 14"}, "", nil,
			&style{font: 42, text: "255 255 255 255", border: "255 0 0 255", bg: "120 0 0 240",
				beam: "Red", icon: "1 Red Square", sound: "2 300"})
	}
	if cfg.HighUncutGems {
		b.section("8.5 UNCUT GEMS (level 20 only)")
		b.rule("Show", []string{`BaseType == "Uncut Skill Gem" "Uncut Spirit Gem"`, "GemLevel >= 20"}, "", nil,
			&style{font: 42, text: "80 255 160 255", border: "0 255 130 255", bg: "5 50 20 255",
				beam: "Green", icon: "1 Green Triangle", sound: "2 300"})
		b.rule("Hide", []string{`BaseType == "Uncut Skill Gem" "Uncut Spirit Gem" "Uncut Support Gem"`}, "", nil, nil)
	}
	if cfg.PinnacleKeys {
		b.section("8.6 PINNACLE KEYS")
		b.rule("Show", []string{`Class == "Pinnacle Keys"`}, "", nil,
			&style{font: 45, text: "255 255 255 255", border: "255 215 0 255", bg: "140 0 170 255",
				beam: "Red", icon: "0 Red Star", sound: "6 300"})
	}

	// ---- 9. below-threshold items ------------------------------------------
	if cfg.FilterMode == "hide" || cfg.FilterMode == "dim" {
		action, dim := "Hide", (*style)(nil)
		if cfg.FilterMode == "dim" {
			action, dim = "Show", styleDim
		}
		b.section(fmt.Sprintf("9. BELOW THRESHOLD (%s)", strings.ToUpper(cfg.FilterMode)))
		var names []string
		for _, c := range cheapCur {
			names = append(names, c.name)
		}
		sort.Strings(names)
		b.rule(action, nil, "BaseType", names, dim)
		b.rule(action, []string{"Rarity == Unique"}, "BaseType", cheapUniqueBases, dim)
		for _, g := range exGroups(cheapEx) {
			sort.Strings(cheapEx[g])
			b.rule(action, []string{"Corrupted False", "Rarity Normal Magic", exCond(g)}, "BaseType", cheapEx[g], dim)
		}
		st.CheapCurrency = len(cheapCur)
	} else {
		st.CheapExcept = 0
	}

	// ---- 10/11. strict equipment cleanup -----------------------------------
	if cfg.IncludeGear {
		// Exceptional items we have not priced (yet) are shown, never hidden.
		b.section("10. EXCEPTIONAL BASES NOT YET PRICED (shown until scanned)")
		b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic", "Sockets >= 2"}, "Class", trade.SocketClasses(2), styleExceptionalUnknown)
		b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic", "Sockets >= 3"}, "Class", trade.SocketClasses(3), styleExceptionalUnknown)
		b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic",
			fmt.Sprintf("Quality >= %d", trade.ExceptionalQualityMin)}, "Class", gearClasses, styleExceptionalUnknown)
		st.UnknownExceptOn = true

		b.section("11. HIDE ALL OTHER NORMAL, MAGIC AND RARE EQUIPMENT")
		b.rule("Hide", []string{"Rarity <= Rare"}, "Class", EquipmentClasses, nil)
	}

	return strings.Join(b.lines, "\n"), st
}

func chunkSlice(items []string, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(items); i += size {
		end := min(i+size, len(items))
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

func quoteItems(items []string) []string {
	res := make([]string, len(items))
	for i, it := range items {
		res[i] = `"` + strings.ReplaceAll(strings.TrimSpace(it), `"`, "") + `"`
	}
	return res
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		res = append(res, it)
	}
	return res
}
