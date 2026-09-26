package filter

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"poe2filter/internal/i18n"
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
	sound            string // PlayAlertSound arguments, e.g. "6 300"
	custom           string // CustomAlertSound file in the filter folder
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
			if st.custom != "" {
				b.add(`    CustomAlertSound "` + st.custom + `" 300`)
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
	styleDivine = &style{font: 45, beam: "Cyan", icon: "0 Cyan Star", sound: "6 300"}
	styleMid    = &style{font: 40, text: "240 220 255 255", border: "180 120 255 255", bg: "70 20 100 230",
		icon: "1 Purple Diamond", sound: "2 300"}
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
	styleWaystone = &style{font: 42, text: "255 255 255 255", border: "255 0 0 255", bg: "120 0 0 240",
		beam: "Red", icon: "1 Red Square", sound: "2 300"}
	// Top-tier rare jewels get the loud look; lower tiers keep the colours
	// but drop the beam and the sound.
	styleRareJewel = &style{font: 42, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
		beam: "Yellow", icon: "1 Yellow Diamond", sound: "2 300"}
	styleRareJewelQuiet = &style{font: 40, text: "255 215 0 255", border: "255 180 0 255", bg: "40 25 0 255",
		icon: "2 Yellow Diamond"}
	styleQuality = &style{font: 40, text: "255 255 255 255", border: "255 215 0 255", bg: "40 30 0 240",
		icon: "1 Yellow Diamond"}
	styleUncutGem = &style{font: 42, text: "80 255 160 255", border: "0 255 130 255", bg: "5 50 20 255",
		beam: "Green", icon: "1 Green Triangle", sound: "2 300"}
	stylePinnacle = &style{font: 45, text: "255 255 255 255", border: "255 215 0 255", bg: "140 0 170 255",
		beam: "Red", icon: "0 Red Star", sound: "6 300"}
	styleDim = &style{font: 18, text: "120 120 120 180", border: "0 0 0 0", bg: "0 0 0 150"}
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
	type exGroup struct {
		kind prices.ExceptionalKind
		min  int
	}
	type valueTier struct {
		group       ItemGroup
		thresholdEx float64
		currency    []string
		uniques     []string
		exceptional map[exGroup][]string
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
	cheapEx := map[exGroup][]string{}
	for _, e := range snap.Exceptional {
		name, ok := canon(e.Base)
		if !ok {
			continue
		}
		g := exGroup{e.Kind, e.Min}
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

	// ---- 1. user hide groups (first, so they really are unconditional) ----
	for _, g := range cfg.ItemGroups {
		if g.GroupMode() != ItemGroupModeHide {
			continue
		}
		var uniqueBases, bases, classes []string
		for _, raw := range g.Items {
			key := strings.ToLower(strings.TrimSpace(raw))
			if base, ok := uniqueToBase[key]; ok {
				// Never hide a base because of one junk unique if another
				// unique on the same base is valuable.
				if ub := snap.UniqueBases[base]; ub.MaxEx >= thr && !strings.EqualFold(ub.TopName, raw) {
					st.Warnings = append(st.Warnings, fmt.Sprintf(
						i18n.T("warn.blacklistSkipped"), raw, base, ub.TopName))
					continue
				}
				uniqueBases = append(uniqueBases, base)
			} else if class, ok := itemClass(raw); ok {
				classes = append(classes, class)
			} else if name, ok := canon(raw); ok {
				bases = append(bases, name)
			} else {
				st.Warnings = append(st.Warnings, fmt.Sprintf(i18n.T("warn.blacklistUnknown"), raw))
			}
		}
		if len(uniqueBases)+len(bases)+len(classes) > 0 {
			b.section(fmt.Sprintf(i18n.T("filter.sec.userHide"), strings.ToUpper(g.Name)))
			b.rule("Hide", []string{"Rarity Unique"}, "BaseType", uniqueBases, nil)
			// A base selection means every item that can drop on that exact base,
			// including uniques. Crafted Runeforged/Runemastered variants have
			// different BaseTypes and therefore remain unaffected.
			b.rule("Hide", nil, "BaseType", bases, nil)
			b.rule("Hide", nil, "Class", classes, nil)
		}
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
		styleDivine.with(dp).withSound(cfg.Sound(GroupDivine)))

	// ---- 3. user value tiers ----------------------------------------------
	// Tiers are written from highest to lowest. The first matching block wins,
	// so a 10-divine drop cannot be caught by a 1-divine tier below it.
	for _, tier := range tiers {
		if len(tier.currency)+len(tier.uniques)+len(tier.exceptional) == 0 {
			continue
		}
		b.section(fmt.Sprintf(i18n.T("filter.sec.valueTier"), strings.ToUpper(tier.group.Name),
			tier.group.ThresholdValue, tier.group.ThresholdUnit, tier.thresholdEx))
		pal, _ := cfg.Palette(tier.group.StyleKey(), ns)
		tierStyle := styleMid.with(pal).withSound(cfg.Sound(tier.group.StyleKey()))
		sort.Strings(tier.currency)
		sort.Strings(tier.uniques)
		b.rule("Show", nil, "BaseType", tier.currency, tierStyle)
		b.rule("Show", []string{"Rarity Unique"}, "BaseType", tier.uniques, tierStyle)
		for _, g := range exGroups(tier.exceptional) {
			sort.Strings(tier.exceptional[g])
			b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic Rare", exCond(g)},
				"BaseType", tier.exceptional[g], tierStyle)
		}
	}

	// ---- 3.5 user whitelist -----------------------------------------------
	// Value tiers come first so even default entries such as Mirror of Kalandra
	// receive the user's highest matching price style. The whitelist still
	// guarantees that anything not covered by a tier is shown prominently.
	if len(cfg.Whitelist) > 0 {
		uniqueBases, bases, classes := resolveShowList(cfg.Whitelist, uniqueToBase, canon)
		if len(uniqueBases)+len(bases)+len(classes) > 0 {
			b.section(i18n.T("filter.sec.whitelist"))
			wl, _ := cfg.Palette(GroupWhitelist, ns)
			wst := styleMax.with(wl).withSound(cfg.Sound(GroupWhitelist))
			b.rule("Show", []string{"Rarity Unique"}, "BaseType", uniqueBases, wst)
			b.rule("Show", nil, "BaseType", bases, wst)
			b.rule("Show", nil, "Class", classes, wst)
		}
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
			apexStyle := &style{font: 45, text: "255 255 255 255",
				border: "255 215 0 255", bg: th.BgT1, beam: th.Beam, icon: "0 " + th.IconColor + " Star", sound: "6 300"}
			highStyle := &style{font: 42, text: th.Text, border: th.Border,
				bg: th.BgT1, beam: th.Beam, icon: "1 " + th.IconColor + " " + th.IconShape, sound: "1 300"}
			// A chosen theme replaces the per-category colours for every category.
			if curPal, custom := cfg.Palette(GroupCurrency, ns); custom {
				apexStyle, highStyle = apexStyle.with(curPal), highStyle.with(curPal)
			}
			if snd := cfg.Sound(GroupCurrency); snd != SoundDefault {
				apexStyle, highStyle = apexStyle.withSound(snd), highStyle.withSound(snd)
			}
			b.add("# --- " + th.Name + " ---")
			b.rule("Show", nil, "BaseType", apex, apexStyle)
			b.rule("Show", nil, "BaseType", high, highStyle)
		}
	}

	// ---- 6. valuable unique bases ------------------------------------------
	if len(valuableUniqueBases) > 0 {
		b.section(i18n.T("filter.sec.unique"))
		up, _ := cfg.Palette(GroupUnique, ns)
		b.rule("Show", []string{"Rarity Unique"}, "BaseType", valuableUniqueBases, styleUnique.with(up).withSound(cfg.Sound(GroupUnique)))
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
			b.rule("Show", []string{"Rarity Normal"}, "BaseType", bases, styleChance.with(cp).withSound(cfg.Sound(GroupChance)))
		}
	}

	// ---- 8. valuable exceptional bases (trade scan) -------------------------
	exPal, _ := cfg.Palette(GroupExceptional, ns)
	if len(valuableEx) > 0 {
		b.section(i18n.T("filter.sec.except"))
		for _, g := range exGroups(valuableEx) {
			sort.Strings(valuableEx[g])
			b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic Rare", exCond(g)},
				"BaseType", valuableEx[g], styleExceptional.with(exPal).withSound(cfg.Sound(GroupExceptional)))
		}
	}

	// ---- 8. rares, jewels, quality, waystones, gems, keys --------------------
	if cfg.T5RareTier != TierOff {
		b.section(fmt.Sprintf(i18n.T("filter.sec.t5rare"), tierLabel(cfg.T5RareTier, "")))
		if cfg.T5RareTier == TierHide {
			b.rule("Hide", []string{"Rarity Rare"}, "Class", gearClasses, nil)
		} else {
			tp, _ := cfg.Palette(GroupT5Rare, ns)
			b.rule("Show", []string{"Rarity Rare", fmt.Sprintf("UnidentifiedItemTier >= %d", cfg.T5RareTier)},
				"Class", gearClasses, styleT5Rare.with(tp).withSound(cfg.Sound(GroupT5Rare)))
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
			st = st.with(jp).withSound(cfg.Sound(GroupRareJewel))
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
			styleQuality.with(qp).withSound(cfg.Sound(GroupQuality)))
	}
	if cfg.WaystoneTier != TierOff {
		b.section(fmt.Sprintf(i18n.T("filter.sec.waystones"), tierLabel(cfg.WaystoneTier, "T")))
		if cfg.WaystoneTier == TierHide {
			b.rule("Hide", []string{`Class == "Waystones"`}, "", nil, nil)
		} else {
			wp, _ := cfg.Palette(GroupWaystone, ns)
			b.rule("Show", []string{`Class == "Waystones"`, fmt.Sprintf("WaystoneTier >= %d", cfg.WaystoneTier)}, "", nil,
				styleWaystone.with(wp).withSound(cfg.Sound(GroupWaystone)))
		}
	}
	// Skill and spirit gems share one slider; support gems have their own
	// because they drop far more often.
	gp, _ := cfg.Palette(GroupUncutGem, ns)
	gemStyle := styleUncutGem.with(gp).withSound(cfg.Sound(GroupUncutGem))
	sp, _ := cfg.Palette(GroupUncutSupport, ns)
	supportStyle := styleUncutGem.with(sp).withSound(cfg.Sound(GroupUncutSupport))
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
			stylePinnacle.with(pp).withSound(cfg.Sound(GroupPinnacle)))
	}

	// ---- 8.7 medium whitelist -----------------------------------------------
	// After the valuable sections, so an item that is valuable anyway keeps
	// its stronger highlight; before the hides, so it is never hidden.
	b.userShowGroups(cfg, ns, uniqueToBase, canon, false)

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
			b.rule(action, []string{"Corrupted False", "Rarity Normal Magic", exCond(g)}, "BaseType", cheapEx[g], dim)
		}
		st.CheapCurrency = len(cheapCur)
	} else {
		st.CheapExcept = 0
	}

	// ---- 10/11. strict equipment cleanup -----------------------------------
	if cfg.IncludeGear {
		// Exceptional items we have not priced (yet) are shown, never hidden.
		b.section(i18n.T("filter.sec.unpriced"))
		unk, _ := cfg.Palette(GroupExceptionalUnknown, ns)
		styleUnk := styleExceptionalUnknown.with(unk).withSound(cfg.Sound(GroupExceptionalUnknown))
		b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic", "Sockets >= 2"}, "Class", trade.SocketClasses(2), styleUnk)
		b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic", "Sockets >= 3"}, "Class", trade.SocketClasses(3), styleUnk)
		b.rule("Show", []string{"Corrupted False", "Rarity Normal Magic",
			fmt.Sprintf("Quality >= %d", trade.ExceptionalQualityMin)}, "Class", gearClasses, styleUnk)
		st.UnknownExceptOn = true

		b.section("11. HIDE ALL OTHER NORMAL, MAGIC AND RARE EQUIPMENT")
		b.rule("Hide", []string{"Rarity Normal Magic Rare"}, "Class", EquipmentClasses, nil)
	}

	return strings.Join(b.lines, "\n"), st
}

// resolveShowList splits a show list into bases shown for uniques only (unique
// names and "|unique" entries) and bases shown for every rarity.
// userShowGroups writes the user's shown groups, in their own order. always
// selects which half to write: the groups that outrank the valuable styles, or
// the ones that come after them.
func (b *builder) userShowGroups(cfg Config, ns map[string]Theme, uniqueToBase map[string]string,
	canon func(string) (string, bool), always bool) {
	for _, g := range cfg.ItemGroups {
		if g.GroupMode() != ItemGroupModeShow || g.Always != always {
			continue
		}
		uniqueBases, bases, classes := resolveShowList(g.Items, uniqueToBase, canon)
		if len(uniqueBases)+len(bases)+len(classes) == 0 {
			continue
		}
		pal, _ := cfg.Palette(g.StyleKey(), ns)
		st := styleMid.with(pal).withSound(cfg.Sound(g.StyleKey()))
		b.section(fmt.Sprintf(i18n.T("filter.sec.userShow"), strings.ToUpper(g.Name)))
		b.rule("Show", []string{"Rarity Unique"}, "BaseType", uniqueBases, st)
		b.rule("Show", nil, "BaseType", bases, st)
		b.rule("Show", nil, "Class", classes, st)
	}
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

func resolveShowList(list []string, uniqueToBase map[string]string, canon func(string) (string, bool)) (uniqueBases, bases, classes []string) {
	for _, raw := range list {
		item, uniqueOnly := ParseListEntry(raw)
		if base, ok := uniqueToBase[strings.ToLower(item)]; ok {
			uniqueBases = append(uniqueBases, base)
		} else if class, ok := itemClass(item); ok {
			classes = append(classes, class)
		} else if name, ok := canon(item); ok {
			if uniqueOnly {
				uniqueBases = append(uniqueBases, name)
			} else {
				bases = append(bases, name)
			}
		}
	}
	return uniqueBases, bases, classes
}

// UniqueOnlySuffix marks a list entry that applies to the unique items of a
// base only, e.g. "Sapphire|unique".
const UniqueOnlySuffix = "|unique"

// ParseListEntry splits a custom list entry into its item name and whether it
// is restricted to uniques.
func ParseListEntry(raw string) (name string, uniqueOnly bool) {
	raw = strings.TrimSpace(raw)
	if n, ok := strings.CutSuffix(raw, UniqueOnlySuffix); ok {
		return strings.TrimSpace(n), true
	}
	return raw, false
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
