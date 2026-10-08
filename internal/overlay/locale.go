package overlay

import (
	"embed"
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Items copied from a game client in another language are turned into the
// English item text first, so everything after the reader (the trade search,
// tiers, pseudo totals, the filter's verdict, crafting) works unchanged. The
// fixed words and the item, unique and gem names come from tables built by
// build/locale/build_locale.py; a modifier line is found in the trade site's
// catalog in that language and written back with the English wording of the
// same stat id.

//go:embed data/locale/*.json
var localeFiles embed.FS

// LocaleCatalog returns the trade catalog in a game language (the app sets
// it; the site's language editions share the English stat ids). Without it,
// or when it fails, modifier lines stay untranslated.
var LocaleCatalog func(lang string) (Catalog, error)

// TradeHosts are the trade site editions per game language.
var TradeHosts = map[string]string{
	"de": "de.pathofexile.com", "fr": "fr.pathofexile.com", "es": "es.pathofexile.com",
	"pt": "br.pathofexile.com", "ru": "ru.pathofexile.com", "ja": "jp.pathofexile.com",
	"ko": "kr.pathofexile.com", "th": "th.pathofexile.com", "zh-Hant": "pathofexile.tw",
}

type localeFile struct {
	Lang     string               `json:"lang"`
	Texts    map[string][2]string `json:"texts"`    // key -> [English, local]
	Patterns map[string][2]string `json:"patterns"` // key -> [English, local] (Go regexp)
	Names    struct {
		Item   [][3]string `json:"item"`   // local, English, category
		Unique [][3]string `json:"unique"` // local, English, base
		Gem    [][3]string `json:"gem"`    // local, English, category
	} `json:"names"`
}

type localName struct{ en, category string }

// Locale is one game language's tables.
type Locale struct {
	Lang     string
	en       map[string]string // key -> English text
	local    map[string]string // key -> local text
	patterns map[string]*regexp.Regexp
	items    map[string]localName // lower local -> English
	uniques  map[string]localName
	gems     map[string]localName
	// the names as written, by their lowercase key
	itemNames, gemNames map[string]string
	// item names longest first, to find the base inside a magic item's name
	itemsByLength []string

	// increased matches the advanced copy's "— 50% Increased" in this language.
	increased *regexp.Regexp
	// rankWord is the header's rank word ("Rang"); any other "(Word: N)" is
	// the tier.
	rankWord string

	statsMu  sync.Mutex
	statsFor int64               // UpdatedAtMs of the catalog the index was built from
	stats    map[string][]string // normalized local wording -> stat ids
}

var (
	localesOnce sync.Once
	locales     []*Locale
)

// Locales returns the embedded game languages.
func Locales() []*Locale {
	localesOnce.Do(func() {
		entries, _ := localeFiles.ReadDir("data/locale")
		for _, e := range entries {
			raw, err := localeFiles.ReadFile(path.Join("data/locale", e.Name()))
			if err != nil {
				continue
			}
			var f localeFile
			if json.Unmarshal(raw, &f) != nil || f.Lang == "" {
				continue
			}
			locales = append(locales, newLocale(f))
		}
	})
	return locales
}

func newLocale(f localeFile) *Locale {
	l := &Locale{Lang: f.Lang, en: map[string]string{}, local: map[string]string{}, patterns: map[string]*regexp.Regexp{},
		items: map[string]localName{}, uniques: map[string]localName{}, gems: map[string]localName{},
		itemNames: map[string]string{}, gemNames: map[string]string{}}
	for k, v := range f.Texts {
		l.en[k], l.local[k] = v[0], v[1]
	}
	for k, v := range f.Patterns {
		if re, err := regexp.Compile(v[1]); err == nil {
			l.patterns[k] = re
		}
	}
	// "^(.*)% erhöht$", "^Augmentation : (.*)%$": the value's place in the
	// words.
	if p, ok := f.Patterns["MODIFIER_INCREASED"]; ok {
		before, after, found := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(p[1], "^"), "$"), "(.*)")
		if found {
			if re, err := regexp.Compile(`^` + before + `(\d+(?:\.\d+)?)` + after + `$`); err == nil {
				l.increased = re
			}
		}
	}
	if p, ok := f.Patterns["MODIFIER_LINE"]; ok {
		if m := rankWordRE.FindStringSubmatch(p[1]); m != nil {
			l.rankWord = strings.TrimSpace(m[1])
		}
	}
	for _, r := range f.Names.Item {
		key := strings.ToLower(r[0])
		if _, ok := l.items[key]; !ok {
			l.items[key] = localName{r[1], r[2]}
			l.itemNames[key] = r[0]
			l.itemsByLength = append(l.itemsByLength, key)
		}
	}
	sort.Slice(l.itemsByLength, func(i, j int) bool { return len(l.itemsByLength[i]) > len(l.itemsByLength[j]) })
	for _, r := range f.Names.Unique {
		if _, ok := l.uniques[strings.ToLower(r[0])]; !ok {
			l.uniques[strings.ToLower(r[0])] = localName{r[1], r[2]}
		}
	}
	for _, r := range f.Names.Gem {
		if _, ok := l.gems[strings.ToLower(r[0])]; !ok {
			l.gems[strings.ToLower(r[0])] = localName{r[1], r[2]}
			l.gemNames[strings.ToLower(r[0])] = r[0]
		}
	}
	return l
}

// DetectLocale returns the language of a copied item from its first lines
// ("Gegenstandsklasse: …", "Seltenheit: …"), or nil for English text.
func DetectLocale(raw string) *Locale {
	head := raw
	if i := strings.Index(raw, "--------"); i >= 0 {
		head = raw[:i]
	}
	if strings.Contains(head, "Item Class: ") || strings.Contains(head, "Rarity: ") {
		return nil
	}
	for _, l := range Locales() {
		for _, key := range []string{"ITEM_CLASS", "RARITY"} {
			if p := l.local[key]; p != "" && strings.Contains(head, p) {
				return l
			}
		}
	}
	return nil
}

// LocaleByLang returns a game language's tables, or nil.
func LocaleByLang(lang string) *Locale {
	for _, l := range Locales() {
		if l.Lang == lang {
			return l
		}
	}
	return nil
}

// PropertyNames maps the listing property names of this language to English
// ("Physischer Schaden" -> "Physical Damage"), for the trade results.
func (l *Locale) PropertyNames() map[string]string {
	out := map[string]string{}
	for _, k := range headerKeys {
		local, en := strings.TrimSuffix(l.local[k], ": "), strings.TrimSuffix(l.en[k], ": ")
		if local != "" && en != "" {
			out[local] = en
		}
	}
	if q := strings.TrimSuffix(l.local["QUALITY"], ": "); q != "" {
		out[q] = "Quality"
	}
	return out
}

// OCRTags are the Windows recognizer languages per game language.
var OCRTags = map[string]string{
	"de": "de-DE", "fr": "fr-FR", "es": "es-ES", "pt": "pt-BR", "ru": "ru-RU", "ja": "ja-JP",
	"ko": "ko-KR", "th": "th-TH", "zh-Hant": "zh-TW",
}

// ScreenWords are this language's tooltip words for reading the screen.
func (l *Locale) ScreenWords() ScreenWords {
	words := ScreenWords{Level: ocrKey(strings.TrimSuffix(l.local["GEM_LEVEL"], ": ")), SupportGems: map[string]bool{}}
	for local, g := range l.gems {
		if g.category == "Support Skill Gem" {
			// Holding Alt the game shows the English name.
			words.SupportGems[l.gemNames[local]] = true
			words.SupportGems[g.en] = true
		}
	}
	return words
}

// GemNames are the gem names as the game writes them in this language.
func (l *Locale) GemNames() []string {
	out := make([]string, 0, len(l.gemNames))
	for _, name := range l.gemNames {
		out = append(out, name)
	}
	return out
}

// NamesFor are this language's names of the given English item names.
func (l *Locale) NamesFor(english []string) []string {
	want := make(map[string]bool, len(english))
	for _, e := range english {
		want[e] = true
	}
	var out []string
	for local, n := range l.items {
		if want[n.en] {
			out = append(out, l.itemNames[local])
		}
	}
	return out
}

// EnglishName is the English name of an item, gem or unique name in this
// language.
func (l *Locale) EnglishName(local string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(local))
	if n, ok := l.items[key]; ok {
		return n.en, true
	}
	if g, ok := l.gems[key]; ok {
		return g.en, true
	}
	if u, ok := l.uniques[key]; ok {
		return u.en, true
	}
	return "", false
}

// headerKeys are the "Key: value" lines whose key is translated and whose
// value is kept (numbers, ranges).
var headerKeys = []string{"ITEM_LEVEL", "GEM_LEVEL", "STACK_SIZE", "SOCKETS", "QUALITY", "PHYSICAL_DAMAGE",
	"ELEMENTAL_DAMAGE", "LIGHTNING_DAMAGE", "COLD_DAMAGE", "FIRE_DAMAGE", "CRIT_CHANCE", "ATTACK_SPEED", "ARMOUR",
	"EVASION", "ENERGY_SHIELD", "RUNIC_WARD", "BLOCK_CHANCE", "CHARM_SLOTS", "BASE_SPIRIT", "WAYSTONE_TIER",
	"RELOAD_SPEED", "AREA_LEVEL", "MAP_TIER", "WAYSTONE_REVIVES", "WAYSTONE_PACK_SIZE", "WAYSTONE_MAGIC_MONSTERS",
	"WAYSTONE_RARE_MONSTERS", "WAYSTONE_DROP_CHANCE", "WAYSTONE_RARITY", "WAYSTONE_MONSTER_RARITY",
	"WAYSTONE_EFFECTIVENESS", "TALISMAN_TIER", "TIMELESS_RADIUS", "GRANTS_SKILL"}

// flagKeys are lines that are a single word or two of state.
var flagKeys = []string{"CORRUPTED", "DOUBLE_CORRUPTED", "MIRRORED", "SANCTIFIED", "FRACTURED_ITEM", "UNMODIFIABLE"}

// categoryClasses turns the item category of the name tables into the copied
// text's "Item Class" (the names the filter and the reader use).
var categoryClasses = map[string]string{
	"Body Armour": "Body Armours", "Helmet": "Helmets", "Gloves": "Gloves", "Boots": "Boots", "Shield": "Shields",
	"Buckler": "Bucklers", "Focus": "Foci", "One Hand Mace": "One Hand Maces", "Two Hand Mace": "Two Hand Maces",
	"Warstaff": "Quarterstaves", "Spear": "Spears", "Bow": "Bows", "Crossbow": "Crossbows", "Ring": "Rings",
	"Talisman": "Talismans", "Amulet": "Amulets", "Belt": "Belts", "Wand": "Wands", "Staff": "Staves",
	"Sceptre": "Sceptres", "Quiver": "Quivers", "Jewel": "Jewels", "Charm": "Charms", "Map": "Waystones",
	"Currency": "Stackable Currency", "Omen": "Omen", "Relic": "Relics", "PinnacleKey": "Pinnacle Keys",
	"VaultKey": "Vault Keys", "MapFragment": "Map Fragments", "TowerAugment": "Tablet",
	"ExpeditionLogbook": "Expedition Logbook", "Flail": "Flails", "Dagger": "Daggers", "Claw": "Claws",
	"One Hand Axe": "One Hand Axes", "Two Hand Axe": "Two Hand Axes", "One Hand Sword": "One Hand Swords",
	"Two Hand Sword": "Two Hand Swords", "SoulCore": "Augment", "Breachstone": "Breachstone",
	"Incubator": "Incubators", "Active Skill Gem": "Skill Gems", "Support Skill Gem": "Support Gems",
}

func (l *Locale) item(name string) (localName, bool) {
	n, ok := l.items[strings.ToLower(strings.TrimSpace(name))]
	return n, ok
}

// baseIn finds the longest item name inside a magic item's name ("Schreinzepter
// des Fuchses").
func (l *Locale) baseIn(name string) (localName, bool) {
	low := strings.ToLower(name)
	for _, key := range l.itemsByLength {
		if strings.Contains(low, key) {
			return l.items[key], true
		}
	}
	return localName{}, false
}

// className is the English item class of a translated base.
func flaskClass(en string) string {
	switch {
	case strings.Contains(en, "Life Flask"):
		return "Life Flasks"
	case strings.Contains(en, "Mana Flask"):
		return "Mana Flasks"
	}
	return ""
}

var (
	localNumberRE = regexp.MustCompile(`[+-]?\d+(?:\.\d+)?(?:\(\s*[+-]?\d+(?:\.\d+)?\s*-\s*[+-]?\d+(?:\.\d+)?\s*\))?`)
)

// statIndex maps a local modifier wording (normalized) to its stat ids.
func (l *Locale) statIndex(local Catalog) map[string][]string {
	l.statsMu.Lock()
	defer l.statsMu.Unlock()
	if l.stats != nil && l.statsFor == local.UpdatedAtMs {
		return l.stats
	}
	index := map[string][]string{}
	for _, g := range local.Stats {
		for _, e := range g.Entries {
			if strings.HasPrefix(e.ID, "pseudo.") {
				continue // a copied line is never a total (some catalogs word them alike)
			}
			key := normalizeStat(e.Text)
			index[key] = append(index[key], e.ID)
		}
	}
	l.stats, l.statsFor = index, local.UpdatedAtMs
	return index
}

// englishStat writes a local modifier line with the English wording of the
// same stat, the line's own numbers (and ranges) in order. ok is false when
// the line matches no stat.
func englishStat(line string, index map[string][]string, english map[string]string) (string, bool) {
	ids := index[normalizeStat(line)]
	for _, id := range ids {
		text, ok := english[id]
		if !ok {
			continue
		}
		text = strings.ReplaceAll(text, " (Local)", "")
		values := localNumberRE.FindAllString(line, -1)
		var b strings.Builder
		n := 0
		for i := 0; i < len(text); i++ {
			if text[i] != '#' {
				b.WriteByte(text[i])
				continue
			}
			v := ""
			if n < len(values) {
				v = values[n]
			}
			n++
			// The English wording may carry the sign outside the number ("+# to").
			if i > 0 && (text[i-1] == '+' || text[i-1] == '-') {
				v = strings.TrimLeft(v, "+-")
			}
			b.WriteString(v)
		}
		return b.String(), true
	}
	return line, false
}

var englishStatsCache struct {
	sync.Mutex
	first *StatGroup
	n     int
	at    int64
	stats map[string]string
}

// cachedEnglishStats is englishStats of the catalog last asked for, kept
// while the same catalog comes back (every item read passes it).
func cachedEnglishStats(catalog Catalog) map[string]string {
	if len(catalog.Stats) == 0 {
		return map[string]string{}
	}
	c := &englishStatsCache
	c.Lock()
	defer c.Unlock()
	if c.stats == nil || c.first != &catalog.Stats[0] || c.n != len(catalog.Stats) || c.at != catalog.UpdatedAtMs {
		c.stats, c.first, c.n, c.at = englishStats(catalog), &catalog.Stats[0], len(catalog.Stats), catalog.UpdatedAtMs
	}
	return c.stats
}

// Warm builds what reading an item in this language needs, so the first item
// does not wait for it.
func (l *Locale) Warm(local, english Catalog) {
	if len(local.Stats) > 0 {
		l.statIndex(local)
	}
	cachedEnglishStats(english)
}

func englishStats(catalog Catalog) map[string]string {
	out := map[string]string{}
	for _, g := range catalog.Stats {
		for _, e := range g.Entries {
			if _, ok := out[e.ID]; !ok {
				out[e.ID] = e.Text
			}
		}
	}
	return out
}

// Translate returns the English item text of a copied local item. local is the
// trade catalog in the item's language and english the English one; either may
// be empty, and then modifier lines stay as they are.
func (l *Locale) Translate(raw string, local, english Catalog) string {
	text, _ := l.TranslateKeeping(raw, local, english)
	return text
}

// TranslateKeeping is Translate that also returns, for every line it
// translated, the line as the game wrote it (English line -> local line), so
// the item can be shown in the player's language.
func (l *Locale) TranslateKeeping(raw string, local, english Catalog) (string, map[string]string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	original := append([]string(nil), lines...)
	kept := map[string]string{}
	index := map[string][]string{}
	if len(local.Stats) > 0 {
		index = l.statIndex(local)
	}
	enStats := cachedEnglishStats(english)

	rarity := ""
	classLine := -1
	titleStart, titleEnd := -1, -1
	inMod := false
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			inMod = false
			continue
		case isSeparator(line):
			inMod = false
			if titleStart >= 0 && titleEnd < 0 {
				titleEnd = i
			}
			continue
		case strings.HasPrefix(line, l.local["ITEM_CLASS"]):
			lines[i] = "Item Class: " + strings.TrimPrefix(line, l.local["ITEM_CLASS"])
			classLine = i
			continue
		case strings.HasPrefix(line, l.local["RARITY"]):
			value := strings.TrimSpace(strings.TrimPrefix(line, l.local["RARITY"]))
			for _, k := range []string{"RARITY_NORMAL", "RARITY_MAGIC", "RARITY_RARE", "RARITY_UNIQUE", "RARITY_GEM",
				"RARITY_CURRENCY", "RARITY_DIVCARD", "RARITY_QUEST"} {
				if strings.EqualFold(value, l.local[k]) {
					value = l.en[k]
					break
				}
			}
			rarity = strings.ToLower(value)
			lines[i] = "Rarity: " + value
			titleStart = i + 1
			continue
		case strings.HasPrefix(line, "{"):
			lines[i] = l.header(line)
			inMod = true
			continue
		}
		// The advanced copy marks some header lines "## ".
		if plain := strings.TrimSpace(strings.TrimLeft(line, "# ")); !inMod && l.translateFixed(lines, i, plain) {
			lines[i] = strings.TrimSuffix(line, plain) + lines[i]
			continue
		}
		if titleStart >= 0 && titleEnd < 0 && i >= titleStart {
			continue // the name block is translated below, once it is complete
		}
		// Socketed rune lines carry no header; the game keeps their "(rune)"
		// mark in English.
		if !inMod {
			if body, ok := strings.CutSuffix(line, " (rune)"); ok {
				if text, ok := englishStat(body, index, enStats); ok {
					lines[i] = text + " (rune)"
				}
			}
			continue
		}
		{
			// A modifier can span two lines; try the pair first.
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next != "" && !isSeparator(next) && !strings.HasPrefix(next, "{") {
					if text, ok := englishStat(line+"\n"+next, index, enStats); ok {
						parts := strings.SplitN(text, "\n", 2)
						lines[i] = parts[0]
						if len(parts) == 2 {
							lines[i+1] = parts[1]
						} else {
							lines[i+1] = ""
						}
						i++
						continue
					}
				}
			}
			body, unscalable := line, ""
			if cut, _, ok := strings.Cut(line, l.local["UNSCALABLE_VALUE"]); ok && l.local["UNSCALABLE_VALUE"] != "" {
				body, unscalable = cut, " — Unscalable Value"
			}
			if text, ok := englishStat(body, index, enStats); ok {
				lines[i] = text + unscalable
			}
		}
	}
	if titleStart >= 0 {
		if titleEnd < 0 {
			titleEnd = len(lines)
		}
		class := l.title(lines[titleStart:titleEnd], rarity)
		if class != "" && classLine >= 0 {
			lines[classLine] = "Item Class: " + class
		}
	}
	for i, line := range lines {
		if line != original[i] && strings.TrimSpace(original[i]) != "" {
			kept[strings.TrimSpace(line)] = strings.TrimSpace(original[i])
		}
	}
	return strings.Join(lines, "\n"), kept
}

// showLocal gives an item read from another language its names and modifier
// lines as the player's game writes them: the copied line where it was
// translated as is, else the stat in the language's catalog wording (merged
// lines, pseudo totals).
func showLocal(item *Item, kept map[string]string, local Catalog) {
	if v, ok := kept[item.Name]; ok {
		item.DisplayName = v
	}
	if v, ok := kept[item.BaseType]; ok {
		item.DisplayBase = v
	}
	for i := range item.Mods {
		m := &item.Mods[i]
		lines := strings.Split(m.Text, "\n")
		var out []string
		for _, line := range lines {
			if v, ok := kept[strings.TrimSpace(line)]; ok {
				out = append(out, v)
			} else if v, ok := kept[strings.TrimSpace(line)+" (rune)"]; ok {
				out = append(out, strings.TrimSuffix(v, " (rune)"))
			}
		}
		if len(out) == len(lines) {
			m.Display = strings.Join(out, "\n")
		} else if m.StatID != "" {
			m.Display = LocalText(local, m.StatID, m.Values)
		}
	}
}

// labels are the game words the item card writes, in this language: the
// property and header names, rarities and states, and the item class as the
// copy wrote it.
func (l *Locale) labels(kept map[string]string, class string) map[string]string {
	out := map[string]string{}
	for _, k := range append(append([]string{"ITEM_LEVEL", "REQUIRES", "RARITY", "QUALITY"}, headerKeys...), flagKeys...) {
		en, local := strings.TrimSuffix(strings.TrimSpace(l.en[k]), ":"), strings.TrimSuffix(strings.TrimSpace(l.local[k]), ":")
		if en != "" && local != "" {
			out[strings.TrimSpace(en)] = strings.TrimSpace(local)
		}
	}
	for _, k := range []string{"RARITY_NORMAL", "RARITY_MAGIC", "RARITY_RARE", "RARITY_UNIQUE", "RARITY_GEM", "RARITY_CURRENCY"} {
		if l.en[k] != "" && l.local[k] != "" {
			out[l.en[k]] = l.local[k]
		}
	}
	// "^Sem Identificação(?:\s*\(Nível…" -> "Sem Identificação"
	if re := l.patterns["UNIDENTIFIED"]; re != nil {
		word, _, _ := strings.Cut(strings.TrimPrefix(re.String(), "^"), "(")
		if word = strings.TrimSuffix(strings.TrimSpace(word), "$"); word != "" && !strings.ContainsAny(word, `\[]`) {
			out["Unidentified"] = word
		}
	}
	if v, ok := kept["Item Class: "+class]; ok {
		out["class"] = strings.TrimSpace(strings.TrimPrefix(v, l.local["ITEM_CLASS"]))
	}
	return out
}

// LocalText writes a stat in a language's catalog wording with the given
// values ("" when the catalog has no such stat).
func LocalText(local Catalog, statID string, values []float64) string {
	for _, g := range local.Stats {
		for _, e := range g.Entries {
			if e.ID != statID {
				continue
			}
			text := strings.ReplaceAll(e.Text, " (Local)", "")
			if !strings.Contains(text, "#") {
				// Some catalogs write the value as "X" ("+X au Mana maxi au total").
				text = placeholderXRE.ReplaceAllString(text, "${1}#")
			}
			var b strings.Builder
			n := 0
			for i := 0; i < len(text); i++ {
				if text[i] != '#' {
					b.WriteByte(text[i])
					continue
				}
				if n < len(values) {
					b.WriteString(strconv.FormatFloat(values[n], 'f', -1, 64))
				}
				n++
			}
			return b.String()
		}
	}
	return ""
}

// translateFixed translates a requirements, "Key: value" or state line in
// place (line without the "## " mark); false when it is none of these.
func (l *Locale) translateFixed(lines []string, i int, line string) bool {
	if p := l.local["REQUIRES"]; p != "" && strings.HasPrefix(line, p) {
		// "Erfordert: Stufe 80, 110 (augmented) Int": only the level word
		// differs; the attribute names are read by no one.
		if word := strings.TrimSuffix(l.local["GEM_LEVEL"], ": "); word != "" && strings.HasPrefix(strings.TrimPrefix(line, p), word+" ") {
			lines[i] = "Requires: Level " + strings.TrimPrefix(strings.TrimPrefix(line, p), word+" ")
			return true
		}
		if re := l.patterns["REQUIRES_LINE"]; re != nil {
			if m := re.FindStringSubmatch(line); m != nil && m[re.SubexpIndex("level")] != "" {
				lines[i] = "Requires: Level " + m[re.SubexpIndex("level")]
				return true
			}
		}
		lines[i] = "Requires: " + strings.TrimPrefix(line, p)
		return true
	}
	// "Qualität (Verteidigungsmodifikatoren): +40%": the kind stays local.
	if p := strings.TrimSuffix(l.local["QUALITY"], ": "); p != "" && strings.HasPrefix(line, p+" (") {
		lines[i] = "Quality " + strings.TrimPrefix(line, p+" ")
		return true
	}
	return l.fixedLine(lines, i, line)
}

// fixedLine translates a "Key: value" line or a state line in place.
func (l *Locale) fixedLine(lines []string, i int, line string) bool {
	for _, k := range headerKeys {
		if p := l.local[k]; p != "" && strings.HasPrefix(line, p) {
			value := strings.TrimPrefix(line, p)
			// "Gewährt Fertigkeit: Stufe 20 Antwort der Sterne"
			if word := strings.TrimSuffix(l.local["GEM_LEVEL"], ": "); k == "GRANTS_SKILL" && word != "" {
				if rest, ok := strings.CutPrefix(value, word+" "); ok {
					level, skill, _ := strings.Cut(rest, " ")
					if g, ok := l.gems[strings.ToLower(skill)]; ok {
						skill = g.en
					}
					value = "Level " + level + " " + skill
				}
			}
			lines[i] = l.en[k] + value
			return true
		}
	}
	for _, k := range flagKeys {
		if p := strings.TrimSpace(l.local[k]); p != "" && strings.EqualFold(line, p) {
			lines[i] = l.en[k]
			return true
		}
	}
	if re := l.patterns["UNIDENTIFIED"]; re != nil && re.MatchString(line) {
		lines[i] = "Unidentified"
		return true
	}
	return false
}

// header turns an advanced copy's modifier header into the English one:
// "{ Präfix-Modifikator "Athletischer" (Level: 1) — Leben }" ->
// "{ Prefix Modifier "Athletischer" (Tier: 1) — Leben }". Languages differ in
// the quotes («…»), the word order (Spanish puts "de fabricación" after the
// name) and the tier's word, so the header is taken apart and written again
// in English order. The affix name and the tags stay local; the reader uses
// neither for the search.
func (l *Locale) header(line string) string {
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "{")), "}"))
	parts := strings.Split(inner, " — ")
	head, tails := parts[0], parts[1:]

	name := ""
	for _, q := range headerQuotes {
		if i := strings.Index(head, q[0]); i >= 0 {
			if j := strings.Index(head[i+len(q[0]):], q[1]); j >= 0 {
				name = strings.TrimSpace(head[i+len(q[0]) : i+len(q[0])+j])
				head = head[:i] + " " + head[i+len(q[0])+j+len(q[1]):]
				break
			}
		}
	}
	tier, rank := "", ""
	head = headerNumberRE.ReplaceAllStringFunc(head, func(m string) string {
		sub := headerNumberRE.FindStringSubmatch(m)
		word := strings.ToLower(strings.TrimSpace(sub[1]))
		if word == "rank" || (l.rankWord != "" && word == strings.ToLower(l.rankWord)) {
			rank = sub[2]
		} else {
			tier = sub[2]
		}
		return " "
	})

	// The kind words, longest first, so "Amélioration de Corruption" is not
	// read as "Amélioration".
	low := strings.ToLower(head)
	has := map[string]bool{}
	for _, k := range headerKinds {
		word := strings.ToLower(strings.Trim(l.local[k], ` :«"「`))
		if word != "" && strings.Contains(low, word) {
			has[k] = true
			low = strings.Replace(low, word, " ", 1)
		}
	}
	var b strings.Builder
	b.WriteString("{ ")
	for _, k := range []string{"FRACTURED_MODIFIER", "DESECRATED_MODIFIER", "CRAFTED_MODIFIER"} {
		if has[k] {
			b.WriteString(l.en[k] + " ")
		}
	}
	kind := ""
	for _, k := range []string{"PREFIX_MODIFIER", "SUFFIX_MODIFIER", "IMPLICIT_MODIFIER", "VAAL_UNIQUE_MODIFIER",
		"UNIQUE_MODIFIER", "CORRUPTED_MODIFIER", "ENCHANT_MODIFIER"} {
		if has[k] {
			kind = l.en[k]
			break
		}
	}
	if kind == "" {
		// A kind the tables do not know (a rune's): as written.
		kind = strings.Join(strings.Fields(head), " ")
	}
	b.WriteString(kind)
	if name != "" {
		b.WriteString(` "` + name + `"`)
	}
	if tier != "" {
		b.WriteString(" (Tier: " + tier + ")")
	}
	if rank != "" {
		b.WriteString(" (Rank: " + rank + ")")
	}
	for _, tail := range tails {
		tail = strings.TrimSpace(tail)
		if l.increased != nil {
			if m := l.increased.FindStringSubmatch(tail); m != nil {
				tail = m[1] + "% Increased"
			}
		}
		b.WriteString(" — " + tail)
	}
	b.WriteString(" }")
	return b.String()
}

// headerKinds are the modifier header's kind words, checked longest first
// (see header).
var headerKinds = []string{"CORRUPTED_MODIFIER", "VAAL_UNIQUE_MODIFIER", "PREFIX_MODIFIER", "SUFFIX_MODIFIER",
	"IMPLICIT_MODIFIER", "UNIQUE_MODIFIER", "ENCHANT_MODIFIER", "CRAFTED_MODIFIER", "FRACTURED_MODIFIER",
	"DESECRATED_MODIFIER"}

// headerQuotes are the quotes around an affix name, per language.
var headerQuotes = [][2]string{{`"`, `"`}, {"«", "»"}, {"「", "」"}, {"“", "”"}}

// placeholderXRE is a value written "X" in a catalog wording.
var placeholderXRE = regexp.MustCompile(`(^|[\s+-])X\b`)

// headerNumberRE is a header's "(Level: 2)", "(Palier : 1)", "(階層：1)".
var headerNumberRE = regexp.MustCompile(`\(\s*([^():：]+?)\s*[:：]\s*(\d+)\s*\)`)

// rankWordRE finds the rank's word in a MODIFIER_LINE pattern.
var rankWordRE = regexp.MustCompile(`\\\(([^()\\]+?) ?[:：] ?\(\?P<rank>`)

// title translates the name lines in place and returns the English item class
// ("" when the base was not found).
func (l *Locale) title(title []string, rarity string) string {
	if len(title) == 0 {
		return ""
	}
	base := func(name string) (localName, bool) {
		name = strings.TrimSpace(name)
		for _, k := range []string{"ITEM_SUPERIOR", "ITEM_EXCEPTIONAL"} {
			if re := l.patterns[k]; re != nil {
				if m := re.FindStringSubmatch(name); m != nil {
					if n, ok := l.item(m[1]); ok {
						if k == "ITEM_EXCEPTIONAL" {
							n.en = "Exceptional " + n.en
						}
						return n, true
					}
				}
			}
		}
		return l.item(name)
	}
	var found localName
	ok := false
	switch {
	case rarity == "gem":
		if g, hit := l.gems[strings.ToLower(strings.TrimSpace(title[0]))]; hit {
			title[0], found, ok = g.en, g, true
		}
	case rarity == "unique" && len(title) >= 2:
		if u, hit := l.uniques[strings.ToLower(strings.TrimSpace(title[0]))]; hit {
			title[0] = u.en
		}
		if n, hit := base(title[1]); hit {
			title[1], found, ok = n.en, n, true
		}
	case rarity == "rare" && len(title) >= 2:
		if n, hit := base(title[1]); hit {
			title[1], found, ok = n.en, n, true
		}
	case rarity == "magic":
		if n, hit := l.baseIn(title[0]); hit {
			title[0], found, ok = n.en, n, true
		}
	default:
		if n, hit := base(title[0]); hit {
			title[0], found, ok = n.en, n, true
		} else if g, hit := l.gems[strings.ToLower(strings.TrimSpace(title[0]))]; hit {
			title[0], found, ok = g.en, g, true
		} else if n, hit := l.baseIn(title[0]); hit {
			title[0], found, ok = n.en, n, true
		}
	}
	if !ok {
		return ""
	}
	if c := flaskClass(found.en); c != "" {
		return c
	}
	return categoryClasses[found.category]
}
