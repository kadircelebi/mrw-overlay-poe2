package filter

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"poe2filter/internal/i18n"
	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
)

const defaultChunkSize = 15

// maxAutoStack is the largest stack the automatic stack rules ask for; a
// currency that needs more to reach the threshold never drops that way.
const maxAutoStack = 50

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
	StackRules       int
	Warnings         []string
}

type style struct {
	font             int
	text, border, bg string
	beam, icon       string
	sound            string // PlayAlertSound id, e.g. "6" or "ShMirror"
	custom           string // CustomAlertSound file in the filter folder
	volume           int    // 0 for MaxSoundVolume
}

type builder struct {
	lines []string
}

// add appends lines. Text the player typed (group names, list entries) ends
// up in some of them, so a line break that slipped through can never start a
// line of its own.
func (b *builder) add(l ...string) {
	for _, s := range l {
		b.lines = append(b.lines, strings.Map(lineRune, s))
	}
}

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
				b.add(fmt.Sprintf("    SetFontSize %d", ClampFontSize(st.font)))
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
			vol := st.volume
			if vol <= 0 {
				vol = MaxSoundVolume
			}
			if st.sound != "" {
				b.add(fmt.Sprintf("    PlayAlertSound %s %d", st.sound, vol))
			}
			if st.custom != "" {
				b.add(fmt.Sprintf(`    CustomAlertSound "%s" %d`, st.custom, vol))
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
	styleDivine = &style{font: 45, beam: "Cyan", icon: "0 Cyan Star", sound: "6"}
	styleMid    = &style{font: 40, text: "240 220 255 255", border: "180 120 255 255", bg: "70 20 100 230",
		icon: "1 Purple Diamond", sound: "2"}
	styleMax = &style{font: 45, text: "255 255 255 255", border: "255 215 0 255", bg: "180 0 0 255",
		beam: "Red", icon: "0 Red Star", sound: "6"}
	styleUnique = &style{font: 44, text: "255 255 255 255", border: "255 100 0 255", bg: "175 40 0 255",
		beam: "Red", icon: "0 Red Star", sound: "6"}
	styleChance = &style{font: 38, text: "0 240 255 255", border: "0 200 255 255", bg: "10 30 50 240",
		icon: "2 Cyan Circle"}
	styleExceptional = &style{font: 42, text: "255 255 255 255", border: "0 210 255 255", bg: "0 40 70 240",
		beam: "Cyan", icon: "1 Cyan Diamond", sound: "2"}
	styleExoticHigh = &style{font: 44, text: "255 255 255 255", border: "0 255 190 255", bg: "0 70 55 255",
		beam: "Green", icon: "0 Green Diamond", sound: "2"}
	styleExoticNormal = &style{font: 40, text: "170 255 220 255", border: "0 190 150 255", bg: "0 40 32 240",
		icon: "1 Green Diamond"}
	styleExceptionalUnknown = &style{font: 36, text: "200 230 255 255", border: "0 150 200 255", bg: "0 25 45 220"}
	styleT5Rare             = &style{font: 40, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
		icon: "2 Yellow Diamond"}
	styleWaystone = &style{font: 42, text: "255 255 255 255", border: "255 0 0 255", bg: "120 0 0 240",
		beam: "Red", icon: "1 Red Square", sound: "2"}
	// Top-tier rare jewels get the loud look; lower tiers keep the colours
	// but drop the beam and the sound.
	styleRareJewel = &style{font: 42, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
		beam: "Yellow", icon: "1 Yellow Diamond", sound: "2"}
	styleRareJewelQuiet = &style{font: 40, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
		icon: "2 Yellow Diamond"}
	styleQuality = &style{font: 40, text: "255 255 255 255", border: "255 215 0 255", bg: "40 30 0 240",
		icon: "1 Yellow Diamond"}
	styleUncutGem = &style{font: 42, text: "80 255 160 255", border: "0 255 130 255", bg: "5 50 20 255",
		beam: "Green", icon: "1 Green Triangle", sound: "2"}
	stylePinnacle = &style{font: 45, text: "255 255 255 255", border: "255 215 0 255", bg: "140 0 170 255",
		beam: "Red", icon: "0 Red Star", sound: "6"}
	styleDim = &style{font: 18, text: "120 120 120 180", border: "0 0 0 0", bg: "0 0 0 150"}
	// styleKeep is the "always show" list: never hidden, but plain, so a cheap
	// entry does not look like a valuable drop. Valuable entries are caught
	// earlier by their value tier or price section.
	styleKeep = &style{font: 38, text: "235 235 235 255", border: "150 150 160 255", bg: "30 30 34 230"}
)

// gearClasses are equipment classes (jewels and flasks excluded).
var gearClasses = []string{
	"Amulets", "Belts", "Body Armours", "Boots", "Bows", "Bucklers", "Crossbows",
	"Foci", "Gloves", "Helmets", "One Hand Maces", "Quarterstaves", "Quivers", "Rings",
	"Sceptres", "Shields", "Spears", "Staves", "Talismans", "Two Hand Maces", "Wands",
}

// classOnlyItems are selectable market entries whose filter identity is a
// Class rather than a BaseType. NeverSink uses the same class names for these
// items; treating them as bases silently drops them from user groups.
var classOnlyItems = map[string]string{
	"expedition logbook": "Expedition Logbook",
}

func itemClass(name string) (string, bool) {
	c, ok := classOnlyItems[strings.ToLower(strings.TrimSpace(name))]
	return c, ok
}

// GenerateDynamicFilterBlock builds the rules injected ahead of the base filter.
// The PoE filter language stops at the first matching block, so ORDER MATTERS:
// explicit user intent first, then valuable drops, then hides, and the blanket
// equipment hide last.
//
// ns holds the NeverSink styles of the base filter for "ns:" themes (may be nil).
func GenerateDynamicFilterBlock(cfg Config, snap *prices.Snapshot, validBases map[string]string, ns map[string]Theme) (string, Stats) {
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
	// An exceptional price is per kind and minimum, and (for the shared scan
	// servers' prices) per item level range: 79-81 and 82+ price apart.
	type exGroup struct {
		kind             prices.ExceptionalKind
		min              int
		minIlvl, maxIlvl int
	}
	type valueTier struct {
		group       ItemGroup
		thresholdEx float64
		currency    []string
		uniques     []string
		exceptional map[exGroup][]string
		gems        []uncutGem
	}
	var tiers []*valueTier
	for _, g := range cfg.ItemGroups {
		if g.GroupMode() != ItemGroupModeValue {
			continue
		}
		t := &valueTier{group: g, thresholdEx: g.ThresholdEx(snap.Rates), exceptional: map[exGroup][]string{}}
		if t.thresholdEx <= thr {
			st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.valueTierBelow"), g.Name, t.thresholdEx, thr))
			continue
		}
		tiers = append(tiers, t)
	}
	// A drop belongs to the highest threshold it reaches, independent of the
	// order in which the user created the value groups.
	sort.SliceStable(tiers, func(i, j int) bool { return tiers[i].thresholdEx > tiers[j].thresholdEx })
	tierFor := func(valueEx float64) *valueTier {
		for _, t := range tiers {
			if valueEx >= t.thresholdEx {
				return t
			}
		}
		return nil
	}

	var valuableCur, cheapCur []cur
	for _, c := range snap.Currency {
		// An uncut gem is priced per kind and level ("Uncut Spirit Gem
		// (Level 20)"), a name no rule can match: it goes to its value tier
		// as a level condition, and is never hidden for its price (the uncut
		// gem sliders decide below the tiers).
		if gem, ok := parseUncutGem(c.Name); ok {
			if c.ValueEx >= thr {
				if tier := tierFor(c.ValueEx); tier != nil {
					tier.gems = append(tier.gems, gem)
				}
			}
			continue
		}
		name, ok := canon(c.Name)
		if !ok {
			continue
		}
		value := c.ValueEx
		// A Divine Orb is worth exactly one divine by definition, a Chaos Orb
		// one chaos. Their listed prices come from another source than the
		// rates the thresholds are converted with (poe2scout 495 ex against
		// poe.ninja's 507.5), and the gap dropped Divine Orb out of the user's
		// own "1 divine" tier.
		if name == "Divine Orb" && snap.Rates.DivineEx > 0 {
			value = snap.Rates.DivineEx
		}
		if name == "Chaos Orb" && snap.Rates.ChaosEx > 0 {
			value = snap.Rates.ChaosEx
		}
		if value >= thr {
			if tier := tierFor(value); tier != nil {
				tier.currency = append(tier.currency, name)
			} else if name != "Divine Orb" {
				valuableCur = append(valuableCur, cur{name, c.Category, c.ValueEx})
			}
		} else if name != "Divine Orb" {
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
			if tier := tierFor(ub.MaxEx); tier != nil {
				tier.uniques = append(tier.uniques, name)
			} else {
				valuableUniqueBases = append(valuableUniqueBases, name)
			}
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
	for _, t := range tiers {
		st.ValuableUniques += len(t.uniques)
	}

	valuableEx := map[exGroup][]string{}
	// bucketFloor is the lowest item level with a price range; below it the
	// scan servers price nothing, and NeverSink decides.
	bucketFloor := 0
	cheapEx := map[exGroup][]string{}
	for _, e := range snap.Exceptional {
		name, ok := canon(e.Base)
		if !ok {
			continue
		}
		g := exGroup{e.Kind, e.Min, e.MinIlvl, e.MaxIlvl}
		if e.MinIlvl > 0 && (bucketFloor == 0 || e.MinIlvl < bucketFloor) {
			bucketFloor = e.MinIlvl
		}
		switch {
		case e.Samples > 0 && e.ValueEx >= thr:
			if tier := tierFor(e.ValueEx); tier != nil {
				tier.exceptional[g] = append(tier.exceptional[g], name)
			} else {
				valuableEx[g] = append(valuableEx[g], name)
			}
			st.ValuableExcept++
		case e.Listings >= minExceptionalListingsToHide && e.Samples >= minExceptionalSamplesToHide:
			cheapEx[g] = append(cheapEx[g], name)
			st.CheapExcept++
		}
	}
	exGroups := func(m map[exGroup][]string) []exGroup {
		var gs []exGroup
		for g := range m {
			gs = append(gs, g)
		}
		sort.Slice(gs, func(i, j int) bool {
			if gs[i].kind != gs[j].kind {
				return gs[i].kind < gs[j].kind
			}
			if gs[i].min != gs[j].min {
				return gs[i].min < gs[j].min
			}
			// An item level range is more specific than none: first match wins.
			if (gs[i].minIlvl > 0) != (gs[j].minIlvl > 0) {
				return gs[i].minIlvl > 0
			}
			return gs[i].minIlvl > gs[j].minIlvl
		})
		return gs
	}
	exCond := func(g exGroup) []string {
		var c []string
		if g.kind == prices.KindQuality {
			c = []string{fmt.Sprintf("Quality >= %d", g.min)}
		} else {
			c = []string{fmt.Sprintf("Sockets >= %d", g.min)}
		}
		if g.minIlvl > 0 {
			c = append(c, fmt.Sprintf("ItemLevel >= %d", g.minIlvl))
		}
		if g.maxIlvl > 0 {
			c = append(c, fmt.Sprintf("ItemLevel <= %d", g.maxIlvl))
		}
		return c
	}
	st.ValuableCurrency = len(valuableCur)
	for _, t := range tiers {
		st.ValuableCurrency += len(t.currency)
	}

	// ---- header -----------------------------------------------------------
	b.add("#==============================================================================",
		"# "+i18n.T("filter.header"),
		"# "+fmt.Sprintf(i18n.T("filter.threshold"), cfg.MinValue, cfg.MinValueUnit, thr),
		"# "+fmt.Sprintf(i18n.T("filter.rate"), divEx),
		"# "+fmt.Sprintf(i18n.T("filter.pricesAt"), snap.GeneratedAt.Local().Format("2006-01-02 15:04")),
		"# "+fmt.Sprintf(i18n.T("filter.writtenAt"), time.Now().Format("2006-01-02 15:04")),
		"# "+fmt.Sprintf(i18n.T("filter.lowValue"), strings.ToUpper(cfg.FilterMode)),
		"#==============================================================================",
		"")

	// ---- 0. user stack groups (more specific than anything below) ---------
	b.userStackGroups(cfg, ns, canon)

	// ---- 0b. the Exotic group (NeverSink's exotic rules + the player's) -----
	// The player keeps these on purpose, so they win over every hide below
	// (their own hide groups and the Alt+H list included). NeverSink's own
	// copies sit after our block, behind the gear hides, so they are written
	// here, in the group's two looks.
	exotic := exoticBases(cfg)
	if cfg.ShowExotics {
		var ex builder
		hp, _ := cfg.Palette(GroupExoticHigh, ns)
		np, _ := cfg.Palette(GroupExoticNormal, ns)
		ex.exoticRules(cfg,
			styleExoticHigh.with(hp).withSound(cfg.Sound(GroupExoticHigh)).withVolume(cfg.Volume(GroupExoticHigh)).withFont(cfg.FontSize(GroupExoticHigh)),
			styleExoticNormal.with(np).withSound(cfg.Sound(GroupExoticNormal)).withVolume(cfg.Volume(GroupExoticNormal)).withFont(cfg.FontSize(GroupExoticNormal)))
		if len(ex.lines) > 0 {
			b.section(i18n.T("filter.sec.exotic"))
			b.add(ex.lines...)
		}
	}

	// ---- 1. user hide groups (first, so they really are unconditional) ----
	for _, g := range cfg.ItemGroups {
		if g.GroupMode() != ItemGroupModeHide {
			continue
		}
		var keep []string
		for _, raw := range g.Items {
			name, _, _ := ParseListEntry(raw)
			if base, ok := uniqueToBase[strings.ToLower(name)]; ok {
				// Never hide a base because of one junk unique if another
				// unique on the same base is valuable.
				if ub := snap.UniqueBases[base]; ub.MaxEx >= thr && !strings.EqualFold(ub.TopName, name) {
					st.Warnings = append(st.Warnings, fmt.Sprintf(
						i18n.T("warn.blacklistSkipped"), raw, base, ub.TopName))
					continue
				}
			}
			keep = append(keep, raw)
		}
		l := resolveList(keep, uniqueToBase, canon)
		for _, raw := range l.unknown {
			st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.blacklistUnknown"), raw))
		}
		for _, base := range l.all {
			if exotic[strings.ToLower(base)] {
				st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.hideGroupExotic"), g.Name, base))
			}
		}
		// A plain base means every rarity, uniques included; say so when that
		// hides a valuable unique ("Utility Belt" hides Mageblood).
		for _, base := range l.all {
			if ub, ok := snap.UniqueBases[base]; ok && ub.MaxEx >= thr {
				st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.hideHidesUnique"), g.Name, base, ub.TopName))
			}
		}
		if !l.empty() {
			b.section(fmt.Sprintf(i18n.T("filter.sec.userHide"), strings.ToUpper(g.Name)))
			// Crafted Runeforged/Runemastered variants have different
			// BaseTypes and therefore remain unaffected.
			b.listRules("Hide", l, nil)
		}
	}

	// Alt+H entries the player wants hidden even when valuable.
	var hardHidden builder
	if hardHidden.hiddenRules(cfg, false, canon, snap, thr, &st) {
		b.section(i18n.T("filter.sec.hiddenHard"))
		b.add(hardHidden.lines...)
	}

	// Explicit hide switches and user lists outrank every price-driven style.
	if cfg.HideExalt {
		b.rule("Hide", []string{`Class == "Stackable Currency"`, `BaseType == "Exalted Orb"`}, "", nil, nil)
	}
	if cfg.HideGold {
		b.rule("Hide", []string{`Class == "Stackable Currency"`, `BaseType == "Gold"`}, "", nil, nil)
	}

	// Groups set to always win go before every price-driven style; the others
	// wait until after them, so a valuable item keeps its stronger highlight.
	b.userShowGroups(cfg, ns, uniqueToBase, canon, true)

	// ---- 2. divine spotlight ----------------------------------------------
	// Divine Orb keeps its own look even when a value tier's price range
	// covers it: the dedicated group is the user's choice for exactly this
	// item. Only an "always win" show group (above) outranks it.
	dp, _ := cfg.Palette(GroupDivine, ns)
	b.section(i18n.T("filter.sec.divine"))
	b.rule("Show", []string{`Class == "Stackable Currency"`, `BaseType == "Divine Orb"`}, "", nil,
		styleDivine.with(dp).withSound(cfg.Sound(GroupDivine)).withVolume(cfg.Volume(GroupDivine)).withFont(cfg.FontSize(GroupDivine)))

	// ---- 3. user value tiers ----------------------------------------------
	// Tiers are written from highest to lowest. The first matching block wins,
	// so a 10-divine drop cannot be caught by a 1-divine tier below it.
	for _, tier := range tiers {
		if len(tier.currency)+len(tier.uniques)+len(tier.exceptional)+len(tier.gems) == 0 {
			continue
		}
		b.section(fmt.Sprintf(i18n.T("filter.sec.valueTier"), strings.ToUpper(tier.group.Name),
			tier.group.ThresholdValue, tier.group.ThresholdUnit, tier.thresholdEx))
		pal, _ := cfg.Palette(tier.group.StyleKey(), ns)
		tierStyle := styleMid.with(pal).withSound(cfg.Sound(tier.group.StyleKey())).withVolume(cfg.Volume(tier.group.StyleKey())).withFont(cfg.FontSize(tier.group.StyleKey()))
		sort.Strings(tier.currency)
		sort.Strings(tier.uniques)
		b.rule("Show", nil, "BaseType", tier.currency, tierStyle)
		b.rule("Show", []string{"Rarity Unique"}, "BaseType", tier.uniques, tierStyle)
		sort.Slice(tier.gems, func(i, j int) bool {
			a, c := tier.gems[i], tier.gems[j]
			return a.kind < c.kind || a.kind == c.kind && a.level < c.level
		})
		for _, g := range tier.gems {
			// No "==" on the base type: the level is part of it (see the
			// uncut gem rules below).
			b.rule("Show", []string{fmt.Sprintf(`BaseType "Uncut %s Gem"`, g.kind), fmt.Sprintf("GemLevel == %d", g.level)}, "", nil, tierStyle)
		}
		for _, g := range exGroups(tier.exceptional) {
			sort.Strings(tier.exceptional[g])
			b.rule("Show", append([]string{"Corrupted False", "Rarity Normal Magic Rare"}, exCond(g)...),
				"BaseType", tier.exceptional[g], tierStyle)
		}
	}

	// currencyStyles is the look of valuable currency of a category: apex for a
	// divine or more, high above the threshold. A chosen theme, sound or size
	// for the currency group replaces the per-category defaults.
	currencyStyles := func(cat string) (*style, *style) {
		th := categoryTheme(cat)
		apex := &style{font: 45, text: "255 255 255 255",
			border: "255 215 0 255", bg: th.BgT1, beam: th.Beam, icon: "0 " + th.IconColor + " Star", sound: "6"}
		high := &style{font: 42, text: th.Text, border: th.Border,
			bg: th.BgT1, beam: th.Beam, icon: "1 " + th.IconColor + " " + th.IconShape, sound: "1"}
		if curPal, custom := cfg.Palette(GroupCurrency, ns); custom {
			apex, high = apex.with(curPal), high.with(curPal)
		}
		if snd := cfg.Sound(GroupCurrency); snd != SoundDefault {
			apex, high = apex.withSound(snd), high.withSound(snd)
		}
		apex, high = apex.withVolume(cfg.Volume(GroupCurrency)), high.withVolume(cfg.Volume(GroupCurrency))
		// A chosen size is the top tier's; the next keeps its step below.
		if size := cfg.FontSize(GroupCurrency); size > 0 {
			apex, high = apex.withFont(size), high.withFont(size-3)
		}
		return apex, high
	}

	// ---- 4. stacks worth the threshold -------------------------------------
	// A currency below the threshold can still drop as a valuable stack: 20
	// Simulacrum Splinters at 7.4 ex are 147 ex. Only items that drop in
	// stacks (cfg.Stacked, from the base filter) get these rules; an orb or a
	// rune drops one at a time. For each stack size where the
	// stack's worth reaches the threshold, a divine or a value tier, it is shown
	// with the look a single item of that worth would get; smaller stacks fall
	// through to the hide below. Stacks above maxAutoStack do not drop, so
	// such rules are left out.
	type stackKey struct {
		n   int
		key string
	}
	stackNames := map[stackKey][]string{}
	stackStyles := map[string]*style{}
	lookFor := func(cat string, worth float64) (string, *style) {
		if tier := tierFor(worth); tier != nil {
			key := "tier:" + tier.group.ID
			if stackStyles[key] == nil {
				pal, _ := cfg.Palette(tier.group.StyleKey(), ns)
				stackStyles[key] = styleMid.with(pal).withSound(cfg.Sound(tier.group.StyleKey())).withVolume(cfg.Volume(tier.group.StyleKey())).withFont(cfg.FontSize(tier.group.StyleKey()))
			}
			return key, stackStyles[key]
		}
		apex, high := currencyStyles(cat)
		if divEx > 0 && worth >= divEx {
			stackStyles["apex:"+cat] = apex
			return "apex:" + cat, apex
		}
		stackStyles["high:"+cat] = high
		return "high:" + cat, high
	}
	breakpoints := []float64{thr}
	if divEx > thr {
		breakpoints = append(breakpoints, divEx)
	}
	for _, t := range tiers {
		breakpoints = append(breakpoints, t.thresholdEx)
	}
	for _, c := range cheapCur {
		if c.ex <= 0 || !cfg.Stacked[c.name] {
			continue
		}
		seen := map[int]bool{}
		for _, level := range breakpoints {
			n := int(math.Ceil(level/c.ex - 1e-9))
			if n < 2 || n > maxAutoStack || seen[n] {
				continue
			}
			seen[n] = true
			key, _ := lookFor(c.cat, float64(n)*c.ex)
			stackNames[stackKey{n, key}] = append(stackNames[stackKey{n, key}], c.name)
		}
	}
	if len(stackNames) > 0 {
		keys := make([]stackKey, 0, len(stackNames))
		for k := range stackNames {
			keys = append(keys, k)
		}
		// Larger stacks first: a stack takes the look of the largest size it
		// reaches.
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].n != keys[j].n {
				return keys[i].n > keys[j].n
			}
			return keys[i].key < keys[j].key
		})
		b.section(i18n.T("filter.sec.stacks"))
		for _, k := range keys {
			sort.Strings(stackNames[k])
			b.rule("Show", []string{fmt.Sprintf("StackSize >= %d", k.n)}, "BaseType", stackNames[k], stackStyles[k.key])
		}
		st.StackRules = len(keys)
	}

	// ---- 5. valuable currency and bulk items -------------------------------
	if len(valuableCur) > 0 {
		b.section(i18n.T("filter.sec.currency"))
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
			th := categoryTheme(cat)
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
			apexStyle, highStyle := currencyStyles(cat)
			b.add("# --- " + th.Name + " ---")
			b.rule("Show", nil, "BaseType", apex, apexStyle)
			b.rule("Show", nil, "BaseType", high, highStyle)
		}
	}

	// ---- 6. valuable unique bases ------------------------------------------
	if len(valuableUniqueBases) > 0 {
		b.section(i18n.T("filter.sec.unique"))
		up, _ := cfg.Palette(GroupUnique, ns)
		b.rule("Show", []string{"Rarity Unique"}, "BaseType", valuableUniqueBases, styleUnique.with(up).withSound(cfg.Sound(GroupUnique)).withVolume(cfg.Volume(GroupUnique)).withFont(cfg.FontSize(GroupUnique)))
	}

	// ---- 7. chance bases ---------------------------------------------------
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
			b.section(i18n.T("filter.sec.chance"))
			// Orb of Chance only works on normal items.
			cp, _ := cfg.Palette(GroupChance, ns)
			b.rule("Show", []string{"Rarity Normal"}, "BaseType", bases, styleChance.with(cp).withSound(cfg.Sound(GroupChance)).withVolume(cfg.Volume(GroupChance)).withFont(cfg.FontSize(GroupChance)))
		}
	}

	// ---- 8. valuable exceptional bases (trade scan) -------------------------
	exPal, _ := cfg.Palette(GroupExceptional, ns)
	if len(valuableEx) > 0 {
		b.section(i18n.T("filter.sec.except"))
		for _, g := range exGroups(valuableEx) {
			sort.Strings(valuableEx[g])
			b.rule("Show", append([]string{"Corrupted False", "Rarity Normal Magic Rare"}, exCond(g)...),
				"BaseType", valuableEx[g], styleExceptional.with(exPal).withSound(cfg.Sound(GroupExceptional)).withVolume(cfg.Volume(GroupExceptional)).withFont(cfg.FontSize(GroupExceptional)))
		}
	}

	// ---- 7c. hidden by me (Alt+H), behind every valuable-item rule ----------
	// Valuable currency, uniques, exceptional and exotic bases were shown
	// above, so these entries hide only while cheap.
	var cheapHidden builder
	if cheapHidden.hiddenRules(cfg, true, canon, snap, thr, &st) {
		b.section(i18n.T("filter.sec.hiddenCheap"))
		b.add(cheapHidden.lines...)
	}

	// ---- 8. rares, jewels, quality, waystones, gems, keys --------------------
	if cfg.T5RareTier != TierOff {
		b.section(fmt.Sprintf(i18n.T("filter.sec.t5rare"), tierLabel(cfg.T5RareTier, "")))
		if cfg.T5RareTier == TierHide {
			b.rule("Hide", []string{"Rarity Rare"}, "Class", gearClasses, nil)
		} else {
			tp, _ := cfg.Palette(GroupT5Rare, ns)
			b.rule("Show", []string{"Rarity Rare", fmt.Sprintf("UnidentifiedItemTier >= %d", cfg.T5RareTier)},
				"Class", gearClasses, styleT5Rare.with(tp).withSound(cfg.Sound(GroupT5Rare)).withVolume(cfg.Volume(GroupT5Rare)).withFont(cfg.FontSize(GroupT5Rare)))
		}
	}
	if cfg.RareJewelTier != TierOff {
		b.section(fmt.Sprintf(i18n.T("filter.sec.jewels"), tierLabel(cfg.RareJewelTier, "")))
		if cfg.RareJewelTier != TierHide {
			// The top tier gets the louder look; anything below is a quieter show.
			st := styleRareJewel
			if cfg.RareJewelTier < MaxRareTier {
				st = styleRareJewelQuiet
			}
			jp, _ := cfg.Palette(GroupRareJewel, ns)
			st = st.with(jp).withSound(cfg.Sound(GroupRareJewel)).withVolume(cfg.Volume(GroupRareJewel)).withFont(cfg.FontSize(GroupRareJewel))
			conds := []string{`Class == "Jewels"`, "Rarity Rare"}
			if cfg.RareJewelTier > 0 {
				conds = append(conds, fmt.Sprintf("UnidentifiedItemTier >= %d", cfg.RareJewelTier))
			}
			b.rule("Show", conds, "", nil, st)
		}
		// Normal/Magic/Rare only: unique jewels follow the unique rules above.
		b.rule("Hide", []string{`Class == "Jewels"`, "Rarity Normal Magic Rare"}, "", nil, nil)
	}
	if cfg.QualityThreshold > 0 {
		b.section(fmt.Sprintf(i18n.T("filter.sec.quality"), cfg.QualityThreshold))
		qp, _ := cfg.Palette(GroupQuality, ns)
		b.rule("Show", []string{"Rarity Normal Magic Rare", fmt.Sprintf("Quality >= %d", cfg.QualityThreshold)}, "Class", gearClasses,
			styleQuality.with(qp).withSound(cfg.Sound(GroupQuality)).withVolume(cfg.Volume(GroupQuality)).withFont(cfg.FontSize(GroupQuality)))
	}
	if cfg.WaystoneTier != TierOff {
		b.section(fmt.Sprintf(i18n.T("filter.sec.waystones"), tierLabel(cfg.WaystoneTier, "T")))
		if cfg.WaystoneTier == TierHide {
			b.rule("Hide", []string{`Class == "Waystones"`}, "", nil, nil)
		} else {
			// From the chosen tier up the waystone is shown, below it hidden,
			// like the gem sliders: the player asked to start there.
			wp, _ := cfg.Palette(GroupWaystone, ns)
			b.rule("Show", []string{`Class == "Waystones"`, fmt.Sprintf("WaystoneTier >= %d", cfg.WaystoneTier)}, "", nil,
				styleWaystone.with(wp).withSound(cfg.Sound(GroupWaystone)).withVolume(cfg.Volume(GroupWaystone)).withFont(cfg.FontSize(GroupWaystone)))
			b.rule("Hide", []string{`Class == "Waystones"`}, "", nil, nil)
		}
	}
	// Skill and spirit gems share one slider; support gems have their own
	// because they drop far more often.
	gp, _ := cfg.Palette(GroupUncutGem, ns)
	gemStyle := styleUncutGem.with(gp).withSound(cfg.Sound(GroupUncutGem)).withVolume(cfg.Volume(GroupUncutGem)).withFont(cfg.FontSize(GroupUncutGem))
	sp, _ := cfg.Palette(GroupUncutSupport, ns)
	supportStyle := styleUncutGem.with(sp).withSound(cfg.Sound(GroupUncutSupport)).withVolume(cfg.Volume(GroupUncutSupport)).withFont(cfg.FontSize(GroupUncutSupport))
	// No "==" here, unlike everywhere else: an uncut gem carries its level in
	// its base type ("Uncut Support Gem (Level 5)"), so an exact match never
	// fires and the rules below would silently do nothing. NeverSink matches
	// them the same way.
	skillGems := `BaseType "Uncut Skill Gem" "Uncut Spirit Gem"`
	supportGems := `BaseType "Uncut Support Gem"`
	if cfg.UncutGemLevel != TierOff || cfg.UncutSupportLevel != TierOff {
		b.section(fmt.Sprintf(i18n.T("filter.sec.gems"), gemLevelLabel(cfg)))
	}
	switch {
	case cfg.UncutGemLevel == TierOff:
	case cfg.UncutGemLevel == TierHide:
		b.rule("Hide", []string{skillGems}, "", nil, nil)
	default:
		b.rule("Show", []string{skillGems, fmt.Sprintf("GemLevel >= %d", cfg.UncutGemLevel)}, "", nil, gemStyle)
		b.rule("Hide", []string{skillGems}, "", nil, nil)
	}
	switch {
	case cfg.UncutSupportLevel == TierOff:
	case cfg.UncutSupportLevel == TierHide:
		b.rule("Hide", []string{supportGems}, "", nil, nil)
	default:
		b.rule("Show", []string{supportGems, fmt.Sprintf("GemLevel >= %d", cfg.UncutSupportLevel)}, "", nil, supportStyle)
		b.rule("Hide", []string{supportGems}, "", nil, nil)
	}
	if cfg.PinnacleKeys {
		b.section(i18n.T("filter.sec.pinnacle"))
		pp, _ := cfg.Palette(GroupPinnacle, ns)
		b.rule("Show", []string{`Class == "Pinnacle Keys"`}, "", nil,
			stylePinnacle.with(pp).withSound(cfg.Sound(GroupPinnacle)).withVolume(cfg.Volume(GroupPinnacle)).withFont(cfg.FontSize(GroupPinnacle)))
	}

	// ---- 8.7 medium whitelist -----------------------------------------------
	// After the valuable sections, so an item that is valuable anyway keeps
	// its stronger highlight; before the hides, so it is never hidden.
	b.userShowGroups(cfg, ns, uniqueToBase, canon, false)

	// ---- 8.8 user "always show" list ----------------------------------------
	// "Always show" means "never hide", not "highlight": an entry that is worth
	// something was already styled by its value tier or price section above;
	// what is left (a cheap catalyst, a junk unique on a Headhunter base) is
	// shown plainly instead of disappearing under the threshold hides below.
	if len(cfg.Whitelist) > 0 {
		if l := resolveList(cfg.Whitelist, uniqueToBase, canon); !l.empty() {
			b.section(i18n.T("filter.sec.whitelist"))
			wl, _ := cfg.Palette(GroupWhitelist, ns)
			wst := styleKeep.with(wl).withSound(cfg.Sound(GroupWhitelist)).withVolume(cfg.Volume(GroupWhitelist)).withFont(cfg.FontSize(GroupWhitelist))
			b.listRules("Show", l, wst)
		}
	}

	// ---- 9. below-threshold items ------------------------------------------
	if cfg.FilterMode == "hide" || cfg.FilterMode == "dim" {
		action, dim := "Hide", (*style)(nil)
		if cfg.FilterMode == "dim" {
			action, dim = "Show", styleDim
		}
		b.section(fmt.Sprintf(i18n.T("filter.sec.below"), strings.ToUpper(cfg.FilterMode)))
		var names []string
		for _, c := range cheapCur {
			names = append(names, c.name)
		}
		sort.Strings(names)
		b.rule(action, nil, "BaseType", names, dim)
		b.rule(action, []string{"Rarity Unique"}, "BaseType", cheapUniqueBases, dim)
		for _, g := range exGroups(cheapEx) {
			sort.Strings(cheapEx[g])
			b.rule(action, append([]string{"Corrupted False", "Rarity Normal Magic"}, exCond(g)...), "BaseType", cheapEx[g], dim)
		}
		st.CheapCurrency = len(cheapCur)
	} else {
		st.CheapExcept = 0
	}

	// ---- 10/11. strict equipment cleanup -----------------------------------
	if cfg.IncludeGear {
		// Exceptional items we have not priced (yet) are shown, never hidden.
		// With the scan servers' prices, which start at item level 79, lower
		// items are not "unpriced": they are not worth pricing.
		b.section(i18n.T("filter.sec.unpriced"))
		unk, _ := cfg.Palette(GroupExceptionalUnknown, ns)
		styleUnk := styleExceptionalUnknown.with(unk).withSound(cfg.Sound(GroupExceptionalUnknown)).withVolume(cfg.Volume(GroupExceptionalUnknown)).withFont(cfg.FontSize(GroupExceptionalUnknown))
		unpriced := func(c string) []string {
			conds := []string{"Corrupted False", "Rarity Normal Magic", c}
			if bucketFloor > 0 {
				conds = append(conds, fmt.Sprintf("ItemLevel >= %d", bucketFloor))
			}
			return conds
		}
		b.rule("Show", unpriced("Sockets >= 2"), "Class", trade.SocketClasses(2), styleUnk)
		b.rule("Show", unpriced("Sockets >= 3"), "Class", trade.SocketClasses(3), styleUnk)
		b.rule("Show", unpriced(fmt.Sprintf("Quality >= %d", trade.ExceptionalQualityMin)), "Class", gearClasses, styleUnk)
		st.UnknownExceptOn = true

		b.section("11. HIDE ALL OTHER NORMAL, MAGIC AND RARE EQUIPMENT")
		b.rule("Hide", []string{"Rarity Normal Magic Rare"}, "Class", EquipmentClasses, nil)
	}

	return strings.Join(b.lines, "\n"), st
}

// userShowGroups writes the user's shown groups, in their own order. always
// selects which half to write: the groups that outrank the valuable styles, or
// the ones that come after them.
func (b *builder) userShowGroups(cfg Config, ns map[string]Theme, uniqueToBase map[string]string,
	canon func(string) (string, bool), always bool) {
	for _, g := range cfg.ItemGroups {
		if g.GroupMode() != ItemGroupModeShow || g.Always != always {
			continue
		}
		l := resolveList(g.Items, uniqueToBase, canon)
		if l.empty() {
			continue
		}
		pal, _ := cfg.Palette(g.StyleKey(), ns)
		st := styleMid.with(pal).withSound(cfg.Sound(g.StyleKey())).withVolume(cfg.Volume(g.StyleKey())).withFont(cfg.FontSize(g.StyleKey()))
		b.section(fmt.Sprintf(i18n.T("filter.sec.userShow"), strings.ToUpper(g.Name)))
		b.listRules("Show", l, st)
	}
}

// userStackGroups writes the show-group entries that ask for a stack size
// ("Simulacrum Splinter|x15", "Verisium|x500"). They are the most specific
// rules the user writes, so they come before everything, hide groups
// included: hiding Simulacrum Splinter while showing stacks of 15 then works.
// Larger stacks go first, so a stack takes the look of the largest size it
// reaches.
func (b *builder) userStackGroups(cfg Config, ns map[string]Theme, canon func(string) (string, bool)) {
	type entry struct {
		group ItemGroup
		n     int
		bases []string
	}
	var entries []entry
	for _, g := range cfg.ItemGroups {
		if g.GroupMode() != ItemGroupModeShow {
			continue
		}
		byStack := map[int][]string{}
		for _, raw := range g.Items {
			item, _, n := ParseListEntry(raw)
			if n == 0 {
				continue
			}
			if name, ok := canon(item); ok {
				byStack[n] = append(byStack[n], name)
			}
		}
		for n, bases := range byStack {
			entries = append(entries, entry{g, n, bases})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].n > entries[j].n })
	for _, e := range entries {
		pal, _ := cfg.Palette(e.group.StyleKey(), ns)
		st := styleMid.with(pal).withSound(cfg.Sound(e.group.StyleKey())).withVolume(cfg.Volume(e.group.StyleKey())).withFont(cfg.FontSize(e.group.StyleKey()))
		sort.Strings(e.bases)
		b.section(fmt.Sprintf(i18n.T("filter.sec.userStack"), strings.ToUpper(e.group.Name), e.n))
		b.rule("Show", []string{fmt.Sprintf("StackSize >= %d", e.n)}, "BaseType", e.bases, st)
	}
}

// categoryTheme is the look of a price category, the plain currency one for
// an unknown category.
func categoryTheme(cat string) CategoryTheme {
	if th, ok := CategoryThemes[cat]; ok {
		return th
	}
	return CategoryThemes["currency"]
}

// tierLabel names a slider position for a section heading: the two stops
// before the numbers read as words, the rest as "3+" or "T14+".
func tierLabel(v int, prefix string) string {
	switch v {
	case TierHide:
		return i18n.T("filter.sec.hidden")
	case TierOff:
		return i18n.T("filter.sec.none")
	}
	return fmt.Sprintf("%s%d+", prefix, v)
}

// gemLevelLabel describes both gem sliders in one heading, e.g. "20+ / hidden".
func gemLevelLabel(cfg Config) string {
	return tierLabel(cfg.UncutGemLevel, "") + " / " + tierLabel(cfg.UncutSupportLevel, "")
}

// itemList is a user list resolved into filter conditions.
type itemList struct {
	unique    []string // bases matched for Rarity Unique only
	nonUnique []string // bases matched for every rarity but unique
	all       []string // bases matched for every rarity
	classes   []string
	unknown   []string // entries that name no item
}

func (l itemList) empty() bool {
	return len(l.unique)+len(l.nonUnique)+len(l.all)+len(l.classes) == 0
}

// listRules writes one rule per kind of entry.
func (b *builder) listRules(action string, l itemList, st *style) {
	b.rule(action, []string{"Rarity Unique"}, "BaseType", l.unique, st)
	b.rule(action, []string{"Rarity Normal Magic Rare"}, "BaseType", l.nonUnique, st)
	b.rule(action, nil, "BaseType", l.all, st)
	b.rule(action, nil, "Class", l.classes, st)
}

// resolveList resolves a user list. A unique name stands for its base with
// uniques only (the filter cannot see unique names); stack entries are left
// to userStackGroups.
func resolveList(list []string, uniqueToBase map[string]string, canon func(string) (string, bool)) itemList {
	var l itemList
	for _, raw := range list {
		item, scope, stack := ParseListEntry(raw)
		if stack > 0 {
			// Written by userStackGroups, before everything else.
			continue
		}
		if base, ok := uniqueToBase[strings.ToLower(item)]; ok {
			l.unique = append(l.unique, base)
		} else if class, ok := itemClass(item); ok {
			l.classes = append(l.classes, class)
		} else if name, ok := canon(item); ok {
			switch scope {
			case ScopeUnique:
				l.unique = append(l.unique, name)
			case ScopeNonUnique:
				l.nonUnique = append(l.nonUnique, name)
			default:
				l.all = append(l.all, name)
			}
		} else {
			l.unknown = append(l.unknown, raw)
		}
	}
	return l
}

// UniqueOnlySuffix marks a list entry that applies to the unique items of a
// base only, e.g. "Sapphire|unique".
const UniqueOnlySuffix = "|unique"

// NonUniqueSuffix marks a list entry that applies to every rarity of a base
// except unique, e.g. "Utility Belt|nonunique" leaves Mageblood alone.
const NonUniqueSuffix = "|nonunique"

// RarityScope says which rarities of a base a list entry covers.
type RarityScope int

const (
	ScopeAll RarityScope = iota
	ScopeUnique
	ScopeNonUnique
)

// StackSuffix marks a show-list entry that matches only stacks of at least
// that many: "Simulacrum Splinter|x15".
const StackSuffix = "|x"

// ParseListEntry splits a custom list entry into its item name, the rarities
// it covers and its minimum stack.
func ParseListEntry(raw string) (name string, scope RarityScope, minStack int) {
	raw = strings.TrimSpace(raw)
	if n, ok := strings.CutSuffix(raw, NonUniqueSuffix); ok {
		return strings.TrimSpace(n), ScopeNonUnique, 0
	}
	if n, ok := strings.CutSuffix(raw, UniqueOnlySuffix); ok {
		return strings.TrimSpace(n), ScopeUnique, 0
	}
	if i := strings.LastIndex(raw, StackSuffix); i > 0 {
		if v, err := strconv.Atoi(raw[i+len(StackSuffix):]); err == nil && v > 0 {
			return strings.TrimSpace(raw[:i]), ScopeAll, min(v, MaxMinStack)
		}
	}
	return raw, ScopeAll, 0
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
		res[i] = `"` + strings.ReplaceAll(CleanText(it), `"`, "") + `"`
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

// uncutGem is an uncut gem price's kind ("Skill", "Spirit", "Support") and
// level.
type uncutGem struct {
	kind  string
	level int
}

var uncutGemPriceRE = regexp.MustCompile(`^Uncut (Skill|Spirit|Support) Gem \(Level (\d+)\)$`)

// parseUncutGem reads an uncut gem price name, "Uncut Spirit Gem (Level 20)".
func parseUncutGem(name string) (uncutGem, bool) {
	m := uncutGemPriceRE.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return uncutGem{}, false
	}
	level, err := strconv.Atoi(m[2])
	if err != nil || level < 1 {
		return uncutGem{}, false
	}
	return uncutGem{m[1], level}, true
}
