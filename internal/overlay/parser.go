package overlay

import (
	"errors"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type ItemProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ItemMod struct {
	Key      string    `json:"key"`
	StatID   string    `json:"statId"`
	Text     string    `json:"text"`
	Type     string    `json:"type"`
	Affix    string    `json:"affix"`
	Name     string    `json:"name"`
	Tier     int       `json:"tier"`
	Values   []float64 `json:"values"`
	Selected bool      `json:"selected"`
	// Tiers lists each affix's tier when several affixes of the same stat
	// were merged into this line ("P1+P1"); empty for a single affix.
	Tiers []int `json:"tiers,omitempty"`
	// Affixes lists each merged affix's side ("prefix", "suffix") in the
	// order of Tiers, so the panel can write "P1+S1".
	Affixes []string `json:"affixes,omitempty"`
	// AltStatIDs are catalog stats with the same wording as StatID (a local
	// or global twin); searches accept any of them through a count group.
	AltStatIDs []string `json:"altStatIds,omitempty"`
	// Negated marks a "reduced" line searched as the site's "increased" stat
	// (or the other way round): Values are negated, and a search keeps the
	// value as the maximum ("at most -29% increased" = "at least 29% reduced").
	Negated bool `json:"negated,omitempty"`
	// WeightStats makes the line a sum the trade site adds up itself: it is
	// searched as a Weighted Sum v2 group of these stats, weight 1 each, with
	// the line's value as the group's minimum. StatID is then only a key.
	WeightStats []string `json:"weightStats,omitempty"`
	// Hidden lines start unselected under a closed "hidden lines" section:
	// the ones a total stands in for, and totals another one makes redundant.
	Hidden bool `json:"hidden,omitempty"`
}

// ParseOptions tune what a parsed item offers for the search.
type ParseOptions struct {
	// SignedIn: GGG refuses Weighted Sum groups without a pathofexile.com
	// session, so sums start selected only with one.
	SignedIn bool
}

type Item struct {
	Raw           string `json:"raw"`
	Class         string `json:"class"`
	Rarity        string `json:"rarity"`
	Name          string `json:"name"`
	BaseType      string `json:"baseType"`
	ItemLevel     int    `json:"itemLevel"`
	RequiredLevel int    `json:"requiredLevel"`
	Quality       int    `json:"quality"`
	// RuneSockets counts the "S" entries of the Sockets line.
	RuneSockets int `json:"runeSockets"`
	// A gem's level and support sockets ("Sockets: G G"); zero on other items.
	GemLevel   int `json:"gemLevel"`
	GemSockets int `json:"gemSockets"`
	// GemCorruption is the part of GemLevel a corruption added ("+1").
	GemCorruption int `json:"gemCorruption"`
	// Exceptional is set when the game prefixed the name with "Exceptional":
	// extra sockets or quality are then what the item is priced by.
	Exceptional bool `json:"exceptional"`
	// StackSize is the count of a stackable item ("Stack Size: 808/5000").
	StackSize    int  `json:"stackSize"`
	Unidentified bool `json:"unidentified"`
	Fractured    bool `json:"fractured"`
	Corrupted    bool `json:"corrupted"`
	// TwiceCorrupted is its own state: a corrupted item corrupted again with
	// another item. It is not also reported as Corrupted.
	TwiceCorrupted bool           `json:"twiceCorrupted"`
	Sanctified     bool           `json:"sanctified"`
	Mirrored       bool           `json:"mirrored"`
	Properties     []ItemProperty `json:"properties"`
	Mods           []ItemMod      `json:"mods"`
	// Exchange is the currency exchange id of an item that trades only there
	// (see exchangeID); such items get a worth card and no search.
	Exchange string `json:"exchange,omitempty"`
}

// Snapshot is kept by the service so windows opened after the hotkey event can
// still render the latest item (or the capture error that explains why not).
type Snapshot struct {
	Item  *Item  `json:"item,omitempty"`
	Error string `json:"error,omitempty"`
	// Mode is "hide" when the item was read with the hide shortcut (Alt+H):
	// the window then offers to hide it instead of pricing it.
	Mode string `json:"mode,omitempty"`
}

var (
	headerRE = regexp.MustCompile(`^\{\s*(?:(Desecrated|Crafted|Fractured)\s+)?(Prefix|Suffix|Implicit|Unique|Rune|Corruption\s+Enhancement|Enhancement)(?:\s+Modifier)?(?:\s+"([^"]+)")?(?:\s+\(Tier:\s*(\d+)\))?`)
	rangeRE  = regexp.MustCompile(`\([^()]*\)`)
	numberRE = regexp.MustCompile(`[+-]?\d+(?:\.\d+)?`)
	spaceRE  = regexp.MustCompile(`\s+`)
	signedRE = regexp.MustCompile(`[+-]#`)
	// Advanced copy prints base rolls; this annotation gives the modifier's
	// effective magnitude, without needing to infer its tags from item quality.
	modifierIncreaseRE = regexp.MustCompile(`—\s*(\d+(?:\.\d+)?)% Increased\s*\}$`)
	// A gem with level bonuses breaks its level down under the Level line.
	gemFromGemRE        = regexp.MustCompile(`^\d+ Levels? from Gem\b`)
	gemFromCorruptionRE = regexp.MustCompile(`^[+-]?\d+ Levels? from Corruption\b`)
	// "an additional" in the catalog, with the plural endings after it.
	anAdditionalRE = regexp.MustCompile(`\ban? additional\b`)
	pluralRE       = regexp.MustCompile(`\b(\w+?)s\b`)
)

// gemClass names the class of a gem copied without its "Item Class:" line
// (the game leaves it out for some gems, e.g. from the Skills panel). The
// tag line under the name tells a support gem from a skill gem.
func gemClass(rest []string) string {
	for _, source := range rest {
		line := strings.TrimSpace(source)
		if line == "" {
			continue
		}
		for _, tag := range strings.Split(line, ",") {
			if strings.TrimSpace(tag) == "Support" {
				return "Support Gems"
			}
		}
		break
	}
	return "Skill Gems"
}

func ParseItem(raw string, catalog Catalog) (Item, error) {
	return ParseItemWith(raw, catalog, ParseOptions{})
}

func ParseItemWith(raw string, catalog Catalog, opts ParseOptions) (Item, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	item := Item{Raw: raw, Properties: []ItemProperty{}, Mods: []ItemMod{}}

	var title []string
	titleStart := -1
	// Weapon damage lines already include quality and local modifiers, so DPS
	// is the average hit times attacks per second (as the trade site computes).
	var physical, elemental, chaos, aps float64
	var gemOwnLevel, gemCorruptLevels int
	for i, source := range lines {
		line := strings.TrimSpace(source)
		plainLine := strings.TrimSpace(strings.TrimLeft(line, "# "))
		// A gem prints its skills' numbers (crit, damage, cost) where an
		// item prints its properties; only level, quality and sockets price it.
		gem := item.Rarity == "gem"
		switch {
		case gem && strings.HasPrefix(plainLine, "Level:"):
			// With level bonuses the line is the level in use, gear included
			// ("Level: 38"); the lines below split it and win when present.
			if gemOwnLevel == 0 {
				item.GemLevel = firstInt(line)
			}
		case gem && gemFromGemRE.MatchString(plainLine):
			// The gem's own level plus its corruption is what the trade site
			// lists; levels from the character's gear stay with the character.
			gemOwnLevel = firstInt(line)
			item.GemLevel = gemOwnLevel + gemCorruptLevels
		case gem && gemFromCorruptionRE.MatchString(plainLine):
			gemCorruptLevels = firstInt(line)
			item.GemLevel = gemOwnLevel + gemCorruptLevels
			item.GemCorruption = gemCorruptLevels
		case gem && strings.HasPrefix(plainLine, "Sockets:"):
			item.GemSockets = len(strings.Fields(strings.TrimPrefix(plainLine, "Sockets:")))
		case strings.HasPrefix(line, "Item Class:"):
			item.Class = strings.TrimSpace(strings.TrimPrefix(line, "Item Class:"))
		case strings.HasPrefix(line, "Rarity:"):
			item.Rarity = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Rarity:")))
			titleStart = i + 1
		case strings.HasPrefix(plainLine, "Item Level:"):
			item.ItemLevel = firstInt(line)
		case strings.HasPrefix(plainLine, "Requires:"):
			// Only "Level N" is the level; a line may list attributes alone
			// ("Requires: 30 (augmented) Str, 30 (augmented) Dex").
			if m := requiredLevelRE.FindStringSubmatch(plainLine); m != nil {
				item.RequiredLevel, _ = strconv.Atoi(m[1])
			}
		case strings.HasPrefix(plainLine, "Quality:") || strings.HasPrefix(plainLine, "Quality ("):
			item.Quality = firstInt(line)
			parts := strings.SplitN(plainLine, ":", 2)
			item.Properties = append(item.Properties, ItemProperty{Name: strings.TrimSpace(parts[0]), Value: propertyValue(line)})
		case strings.EqualFold(plainLine, "Unidentified"):
			item.Unidentified = true
		case strings.EqualFold(plainLine, "Fractured Item"):
			item.Fractured = true
		case strings.EqualFold(plainLine, "Corrupted"):
			item.Corrupted = true
		case strings.EqualFold(plainLine, "Twice Corrupted"):
			item.TwiceCorrupted = true
		case strings.EqualFold(plainLine, "Sanctified"):
			item.Sanctified = true
		case strings.EqualFold(plainLine, "Mirrored"):
			item.Mirrored = true
		case gem:
			// Nothing else of a gem's text is a searchable property.
		case weaponDamageKind(line) != "":
			avg := averageDamage(line)
			switch weaponDamageKind(line) {
			case "Physical":
				physical += avg
			case "Chaos":
				chaos += avg
			default:
				elemental += avg
			}
			parts := strings.SplitN(line, ":", 2)
			item.Properties = append(item.Properties, ItemProperty{Name: parts[0], Value: strings.TrimSpace(parts[1])})
		case strings.HasPrefix(line, "Critical Hit Chance:") || strings.HasPrefix(line, "Attacks per Second:"):
			parts := strings.SplitN(line, ":", 2)
			item.Properties = append(item.Properties, ItemProperty{Name: parts[0], Value: strings.TrimSpace(parts[1])})
			if parts[0] == "Attacks per Second" {
				aps = firstFloat(parts[1])
			}
		case strings.HasPrefix(plainLine, "Stack Size:"):
			item.StackSize = stackSize(strings.TrimPrefix(plainLine, "Stack Size:"))
		case strings.HasPrefix(plainLine, "Sockets:"):
			item.RuneSockets = len(strings.Fields(strings.TrimPrefix(plainLine, "Sockets:")))
		case strings.HasPrefix(line, "Armour:") || strings.HasPrefix(line, "Evasion Rating:") || strings.HasPrefix(line, "Energy Shield:") || strings.HasPrefix(line, "Spirit:") || strings.HasPrefix(line, "Runic Ward:"):
			parts := strings.SplitN(line, ":", 2)
			item.Properties = append(item.Properties, ItemProperty{Name: parts[0], Value: strings.TrimSpace(parts[1])})
		case item.Class == "Waystones" && waystoneProperties[strings.SplitN(plainLine, ":", 2)[0]]:
			// A waystone is priced by these totals, not by its modifiers.
			parts := strings.SplitN(plainLine, ":", 2)
			value := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "(augmented)"))
			item.Properties = append(item.Properties, ItemProperty{Name: parts[0], Value: value})
		}
	}
	if aps > 0 && physical+elemental+chaos > 0 {
		if physical > 0 {
			item.Properties = append(item.Properties, ItemProperty{Name: "Physical DPS", Value: formatDPS(physical * aps)})
		}
		if elemental > 0 {
			item.Properties = append(item.Properties, ItemProperty{Name: "Elemental DPS", Value: formatDPS(elemental * aps)})
		}
		item.Properties = append(item.Properties, ItemProperty{Name: "DPS", Value: formatDPS((physical + elemental + chaos) * aps)})
	}
	if titleStart >= 0 {
		for j, source := range lines[titleStart:] {
			line := strings.TrimSpace(source)
			if isSeparator(line) {
				if len(title) > 0 {
					if item.Class == "" && item.Rarity == "gem" {
						item.Class = gemClass(lines[titleStart+j+1:])
					}
					break
				}
				continue
			}
			// Items the character cannot use carry a warning before their name.
			if line != "" && !isUsabilityWarning(line) {
				title = append(title, line)
			}
		}
	}
	if len(title) == 0 || item.Class == "" || item.Rarity == "" {
		return Item{}, errors.New("clipboard does not contain a Path of Exile 2 item")
	}
	if item.Rarity == "unique" && item.Unidentified && len(title) == 1 {
		// An unidentified unique exposes only its base type. Treating that line
		// as the unique name makes the trade API reject the query as unknown.
		item.BaseType = title[0]
	} else if item.Rarity == "magic" && len(title) == 1 {
		// A magic item prints one line, affix names around the base:
		// "Collector's Delirium Tablet of the Essence". The trade site knows
		// only the base, so it is picked out of the line.
		item.Name = title[0]
		item.BaseType = magicBase(title[0], catalog)
	} else if item.Rarity == "unique" || item.Rarity == "rare" || item.Rarity == "magic" {
		item.Name = title[0]
		if len(title) > 1 {
			item.BaseType = title[1]
		}
	} else {
		item.BaseType = title[len(title)-1]
	}
	if item.BaseType == "" {
		item.BaseType = title[len(title)-1]
	}
	if plain := stripExceptional(item.BaseType, catalog); plain != item.BaseType {
		item.BaseType, item.Exceptional = plain, true
	}

	var current *ItemMod
	var currentIncrease float64
	sawHeader := false
	prefixes, suffixes := 0, 0
	commit := func() {
		if current == nil {
			return
		}
		// One modifier header can carry several stats ("41% increased Energy
		// Shield" + "+44 to maximum Life"); the trade site filters each one
		// separately. A few stats genuinely span two lines, so the longest run
		// of lines that matches a catalog stat exactly stays together.
		for _, text := range splitStats(strings.TrimSpace(current.Text), current.Type, catalog) {
			mod := *current
			mod.Text = text
			mod.Values = valuesOf(text)
			matchMod(&mod, catalog, localStatClasses[item.Class])
			mod.Key = "mod-" + strconv.Itoa(len(item.Mods)+1)
			// Runes can be swapped out, so they do not describe the item's value
			// the way its own modifiers (and corruption enchants) do. A
			// waystone is searched by its totals; its modifiers stay optional.
			mod.Selected = mod.StatID != "" && mod.Type != "rune" && item.Class != "Waystones"
			item.Mods = append(item.Mods, mod)
		}
		current = nil
		currentIncrease = 0
	}
	for _, source := range lines {
		line := strings.TrimSpace(source)
		plainLine := strings.TrimSpace(strings.TrimLeft(line, "# "))
		if strings.EqualFold(plainLine, "Unidentified") || strings.EqualFold(plainLine, "Fractured Item") || strings.EqualFold(plainLine, "Corrupted") || strings.EqualFold(plainLine, "Twice Corrupted") || strings.EqualFold(plainLine, "Sanctified") || strings.EqualFold(plainLine, "Mirrored") {
			commit()
			continue
		}
		if m := headerRE.FindStringSubmatch(line); m != nil {
			commit()
			sawHeader = true
			current = &ItemMod{Name: m[3]}
			if strings.Contains(line, "% Increased") {
				if increase := modifierIncreaseRE.FindStringSubmatch(line); increase != nil {
					currentIncrease, _ = strconv.ParseFloat(increase[1], 64)
				}
			}
			if m[4] != "" {
				current.Tier, _ = strconv.Atoi(m[4])
			}
			special, kind := strings.ToLower(m[1]), strings.ToLower(spaceRE.ReplaceAllString(m[2], " "))
			switch special {
			case "crafted", "desecrated", "fractured":
				current.Type = special
			default:
				switch kind {
				case "implicit":
					current.Type = "implicit"
				case "rune":
					current.Type = "rune"
				case "enhancement", "corruption enhancement":
					current.Type = "enchant"
				default:
					current.Type = "explicit"
				}
			}
			if kind == "prefix" || kind == "suffix" {
				current.Affix = kind
				// One header is one affix slot, however many stat lines it
				// carries (a hybrid or desecrated mod can have two or three).
				if kind == "prefix" {
					prefixes++
				} else {
					suffixes++
				}
			}
			continue
		}
		if current == nil {
			// Granted skills have no modifier header of their own.
			if strings.HasPrefix(line, "Grants Skill:") {
				current = &ItemMod{Type: "skill", Text: line}
				commit()
				continue
			}
			if strings.HasSuffix(strings.ToLower(line), "(rune)") {
				text := strings.TrimSpace(line[:len(line)-len("(rune)")])
				current = &ItemMod{Type: "rune", Text: text}
				commit()
			}
			continue
		}
		if line == "" || isSeparator(line) {
			commit()
			continue
		}
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "##") {
			continue
		}
		text, _, unscalable := strings.Cut(line, " — Unscalable Value")
		line = strings.TrimSpace(text)
		if currentIncrease != 0 && !unscalable {
			line = scaleClipboardModifier(line, currentIncrease)
		}
		if current.Text != "" {
			current.Text += "\n"
		}
		current.Text += line
	}
	commit()
	item.Mods = mergeSameStats(item.Mods)
	if !item.Unidentified {
		addPseudoTotals(&item, opts)
	}
	if sawHeader && !item.Unidentified {
		addEmptyAffixes(&item, prefixes, suffixes)
	}
	// Merging same-stat lines leaves gaps in the keys, and lines added after it
	// could take a key still in use; the panel lists mods by key, and a repeated
	// key stops it from drawing the new item at all.
	for i := range item.Mods {
		item.Mods[i].Key = "mod-" + strconv.Itoa(i+1)
	}
	// An uncut gem names its level in its base type ("Uncut Spirit Gem
	// (Level 20)"); the loot filter's rules test it as GemLevel.
	if m := uncutGemLevelRE.FindStringSubmatch(item.BaseType); m != nil && item.GemLevel == 0 {
		item.GemLevel, _ = strconv.Atoi(m[1])
	}
	item.Exchange = catalog.exchangeID(item)
	return item, nil
}

var uncutGemLevelRE = regexp.MustCompile(`^Uncut (?:Skill|Spirit|Support) Gem \(Level (\d+)\)$`)

// scaleClipboardModifier converts the advanced copy's base rolls (including
// roll ranges) to displayed values. Round toward zero at the printed precision,
// as for +3 * 1.4 -> +4. Unannotated/already augmented lines never call this.
func scaleClipboardModifier(text string, increase float64) string {
	factor := (100 + increase) / 100
	return numberRE.ReplaceAllStringFunc(text, func(number string) string {
		value, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return number
		}
		precision := 1.0
		if dot := strings.IndexByte(number, '.'); dot >= 0 {
			precision = math.Pow10(len(number) - dot - 1)
		}
		// Tolerance keeps an exact integer boundary from losing one to binary
		// floating point (e.g. 50 * 1.4).
		value = math.Copysign(math.Trunc(math.Abs(value)*factor*precision+1e-9)/precision, value)
		result := strconv.FormatFloat(value, 'f', -1, 64)
		if strings.HasPrefix(number, "+") && value >= 0 {
			result = "+" + result
		}
		return result
	})
}

// itemSearchGroups are exchange sections whose items are still priced by the
// item search: waystones by tier.
var itemSearchGroups = map[string]bool{"Waystones": true}

// exchangeID is the trade id of an item that trades only on the in-game
// currency exchange (boss keys, splinters, omens, currency...), or "".
func (c Catalog) exchangeID(item Item) string {
	switch item.Rarity {
	case "magic", "rare", "unique":
		return ""
	}
	name := item.BaseType
	if name == "" {
		name = item.Name
	}
	// Logbooks are on the exchange too, but their modifiers set the price.
	if name == "" || strings.EqualFold(name, "Expedition Logbook") {
		return ""
	}
	// Uncut skill and spirit gems trade on the currency exchange and the price
	// list has them by level; support gems are not priced there, so they keep
	// the item search.
	if strings.HasPrefix(name, "Uncut Support Gem") {
		return ""
	}
	for _, e := range c.Currencies {
		if strings.EqualFold(e.Text, name) && !itemSearchGroups[e.Group] {
			return e.ID
		}
	}
	return ""
}

// Affix slots on each side (prefix and suffix) by rarity: magic 1, rare 3,
// and rare jewels and tablets 2. Special crafts and corruption can add a slot
// past these; such an item shows no empty slot, since it is full by the usual
// count.
func affixSlots(item *Item) int {
	class := strings.ToLower(item.Class)
	switch {
	case strings.Contains(class, "waystone"):
		// Waystones are searched by their totals, not by room for mods.
		return 0
	case item.Rarity == "magic":
		return 1
	case item.Rarity == "rare" && (strings.Contains(class, "jewel") || strings.Contains(class, "tablet")):
		return 2
	case item.Rarity == "rare":
		return 3
	}
	return 0
}

const (
	emptyPrefixStat = "pseudo.pseudo_number_of_empty_prefix_mods"
	emptySuffixStat = "pseudo.pseudo_number_of_empty_suffix_mods"
)

// addEmptyAffixes offers the open prefix and suffix slots as the trade site's
// pseudo stats, unselected, so a crafting base can be priced by room left.
// Only the advanced copy (with modifier headers) tells prefixes from suffixes.
func addEmptyAffixes(item *Item, prefixes, suffixes int) {
	slots := affixSlots(item)
	add := func(count int, statID, text, affix string) {
		if count <= 0 {
			return
		}
		item.Mods = append(item.Mods, ItemMod{
			Key: "mod-" + strconv.Itoa(len(item.Mods)+1), StatID: statID, Type: "pseudo", Affix: affix,
			Text: strconv.Itoa(count) + text, Values: []float64{float64(count)},
		})
	}
	add(slots-prefixes, emptyPrefixStat, " Empty Prefix Modifiers", "prefix")
	add(slots-suffixes, emptySuffixStat, " Empty Suffix Modifiers", "suffix")
}

// splitStats groups the lines of one modifier into stats: greedily the
// longest run of lines that is a single catalog stat, otherwise one line each.
func splitStats(text, modType string, catalog Catalog) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		return lines
	}
	var out []string
	for i := 0; i < len(lines); {
		end := i + 1
		for j := len(lines); j > i+1; j-- {
			if exactStat(strings.Join(lines[i:j], "\n"), modType, catalog) != "" {
				end = j
				break
			}
		}
		out = append(out, strings.Join(lines[i:end], "\n"))
		i = end
	}
	return out
}

func catalogType(modType string) string {
	if modType == "rune" {
		// The trade catalog files socketed rune stats under "augment".
		return "augment"
	}
	return modType
}

func exactStat(text, modType string, catalog Catalog) string {
	want, wantType := normalizeStat(text), catalogType(modType)
	for gi := range catalog.Stats {
		for _, e := range catalog.Stats[gi].Entries {
			if (wantType == "" || e.Type == wantType) && normalizeStat(e.Text) == want {
				return e.ID
			}
		}
	}
	return ""
}

// magicBase finds the catalog base type inside a magic item's name; the
// longest one wins, so "Time-Lost Emerald" beats "Emerald". Without a match
// the whole line is kept, as before.
func magicBase(line string, catalog Catalog) string {
	best := ""
	padded := " " + line + " "
	for _, group := range catalog.Items {
		for _, e := range group.Entries {
			if e.Name != "" || len(e.Type) <= len(best) {
				continue
			}
			if strings.Contains(padded, " "+e.Type+" ") {
				best = e.Type
			}
		}
	}
	if best == "" {
		return line
	}
	return best
}

// matchMod finds the trade stat of an item line. The catalog often has the
// same wording twice, a global stat and a "(Local)" one; weapons and armour
// roll the local one (their own armour, damage, attack speed), everything
// else the global one. Searching the wrong one finds nothing: a shield's
// "increased Armour" searched as the global stat matched no Svalinn at all.
// The other stats of the same wording are kept as AltStatIDs, so a search
// can accept any of them (the catalog also has twins with no "(Local)" mark,
// like the two "# to maximum Runic Ward").
func matchMod(mod *ItemMod, catalog Catalog, local bool) {
	want := normalizeStat(mod.Text)
	wantType := catalogType(mod.Type)
	exact, fallback := catalogMatches(want, wantType, catalog)
	if len(exact) == 0 {
		// The site lists some stats only one way round: "29% reduced Tribute"
		// is searched as "-29% increased Tribute".
		for _, swap := range [][2]string{{"reduced", "increased"}, {"increased", "reduced"}, {"less", "more"}, {"more", "less"}} {
			if !strings.Contains(want, swap[0]) {
				continue
			}
			if swapped, _ := catalogMatches(strings.Replace(want, swap[0], swap[1], 1), wantType, catalog); len(swapped) > 0 {
				exact = swapped
				mod.Negated = true
				for i := range mod.Values {
					mod.Values[i] = -mod.Values[i]
				}
				break
			}
		}
	}
	if len(exact) == 0 {
		if fallback != nil {
			setMatchedStat(mod, fallback)
			return
		}
		// Some lines the game prints as implicits are searched as pseudo
		// stats: a tablet's "6 uses remaining" is "# uses remaining (Tablets)".
		if wantType != "pseudo" {
			if id := pseudoFor(want, catalog); id != "" {
				mod.StatID = id
			}
		}
		return
	}
	primary := 0
	for i, e := range exact {
		if strings.Contains(e.Text, "(Local)") == local {
			primary = i
			break
		}
	}
	setMatchedStat(mod, exact[primary])
	for i, e := range exact {
		if i != primary {
			mod.AltStatIDs = append(mod.AltStatIDs, e.ID)
		}
	}
}

// Literal numbers in a presence-only stat (Headhunter's 60 seconds) are
// description text, not a searchable roll. Option stats remain counts.
func setMatchedStat(mod *ItemMod, entry *StatEntry) {
	mod.StatID = entry.ID
	if !strings.Contains(entry.Text, "#") && !strings.Contains(entry.ID, "|") {
		mod.Values = nil
	}
}

// catalogMatches returns the stats worded exactly like want, and else the
// first one whose wording contains it (or is contained in it).
func catalogMatches(want, wantType string, catalog Catalog) ([]*StatEntry, *StatEntry) {
	var fallback *StatEntry
	var exact []*StatEntry
	for gi := range catalog.Stats {
		for ei := range catalog.Stats[gi].Entries {
			e := &catalog.Stats[gi].Entries[ei]
			if wantType != "" && e.Type != wantType {
				continue
			}
			got := normalizeStat(e.Text)
			if got == want {
				if !slices.ContainsFunc(exact, func(x *StatEntry) bool { return x.ID == e.ID }) {
					exact = append(exact, e)
				}
				continue
			}
			if fallback == nil && (strings.Contains(want, got) || strings.Contains(got, want)) {
				fallback = e
			}
		}
	}
	return exact, fallback
}

func pseudoFor(want string, catalog Catalog) string {
	for gi := range catalog.Stats {
		for _, e := range catalog.Stats[gi].Entries {
			if e.Type == "pseudo" && normalizeStat(e.Text) == want {
				return e.ID
			}
		}
	}
	return ""
}

// localStatClasses are the item classes whose modifiers are local to the
// item: weapons and armour pieces. Jewellery, quivers, jewels, flasks and
// the rest roll global stats.
var localStatClasses = map[string]bool{
	"Claws": true, "Daggers": true, "One Hand Swords": true, "One Hand Axes": true, "One Hand Maces": true,
	"Spears": true, "Flails": true, "Two Hand Swords": true, "Two Hand Axes": true, "Two Hand Maces": true,
	"Quarterstaves": true, "Talismans": true, "Bows": true, "Crossbows": true, "Wands": true, "Sceptres": true,
	"Staves": true, "Helmets": true, "Body Armours": true, "Gloves": true, "Boots": true, "Shields": true,
	"Foci": true, "Bucklers": true,
}

func normalizeStat(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " (Local)", "")
	s = rangeRE.ReplaceAllString(s, "")
	s = numberRE.ReplaceAllString(s, "#")
	// The catalog sometimes keeps the sign outside the placeholder ("have +#
	// Cooldown Use") while the item's "+1" became "#" together with its sign.
	s = signedRE.ReplaceAllString(s, "#")
	s = strings.ToLower(s)
	// The catalog names a count of one ("Map contains an additional Rare
	// Chest"); the game prints the rolled count in the plural ("2 additional
	// Rare Chests"). Both become "# additional rare chest".
	s = anAdditionalRE.ReplaceAllString(s, "# additional")
	if i := strings.Index(s, "# additional "); i >= 0 {
		s = s[:i] + pluralRE.ReplaceAllString(s[i:], "$1")
	}
	s = strings.ReplaceAll(s, "slots", "slot")
	s = strings.ReplaceAll(s, "damageable companion's", "damageable companion")
	s = strings.ReplaceAll(s, "’", "'")
	s = spaceRE.ReplaceAllString(s, "")
	return s
}

func valuesOf(s string) []float64 {
	withoutRanges := rangeRE.ReplaceAllString(s, "")
	matches := numberRE.FindAllString(withoutRanges, -1)
	values := make([]float64, 0, len(matches))
	for _, m := range matches {
		if v, err := strconv.ParseFloat(strings.TrimPrefix(m, "+"), 64); err == nil {
			values = append(values, v)
		}
	}
	return values
}

var damageRangeRE = regexp.MustCompile(`(\d+(?:\.\d+)?)-(\d+(?:\.\d+)?)`)

// weaponDamageKind returns the damage type of a weapon property line such as
// "Lightning Damage: 4-233 (lightning)", or "" for any other line.
func weaponDamageKind(line string) string {
	for _, kind := range []string{"Physical", "Fire", "Cold", "Lightning", "Chaos"} {
		if strings.HasPrefix(line, kind+" Damage:") {
			return kind
		}
	}
	return ""
}

// averageDamage sums the average of every "min-max" range on the line.
func averageDamage(line string) float64 {
	var sum float64
	for _, m := range damageRangeRE.FindAllStringSubmatch(line, -1) {
		lo, _ := strconv.ParseFloat(m[1], 64)
		hi, _ := strconv.ParseFloat(m[2], 64)
		sum += (lo + hi) / 2
	}
	return sum
}

func firstFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimPrefix(numberRE.FindString(s), "+"), 64)
	return v
}

func formatDPS(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func isUsabilityWarning(s string) bool {
	return strings.HasPrefix(s, "You cannot use this item")
}

func isSeparator(s string) bool {
	return len(s) >= 5 && strings.Trim(s, "-") == ""
}

func firstInt(s string) int {
	m := numberRE.FindString(s)
	v, _ := strconv.Atoi(strings.TrimPrefix(m, "+"))
	return v
}

func propertyValue(s string) string {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// stackSize reads the count before the slash; the game may group thousands
// ("1,234/5,000").
func stackSize(value string) int {
	count, _, _ := strings.Cut(value, "/")
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, count)
	n, _ := strconv.Atoi(digits)
	return n
}

// stripExceptional turns "Exceptional Hawker's Jacket" into the real base: the
// game prefixes the name of an exceptional item, but the trade site only knows
// the plain base and rejects the prefixed one. A name the catalog knows as it
// is ("Exceptional Verisium" is a currency) is left alone.
func stripExceptional(base string, catalog Catalog) string {
	const prefix = "Exceptional "
	if !strings.HasPrefix(base, prefix) {
		return base
	}
	plain := strings.TrimPrefix(base, prefix)
	known := func(name string) bool {
		for _, group := range catalog.Items {
			for _, entry := range group.Entries {
				if entry.Name == "" && entry.Type == name {
					return true
				}
			}
		}
		return false
	}
	if known(base) || !known(plain) {
		return base
	}
	return plain
}

// ListingStatID finds the trade stat of a listing's mod line for sorting.
// Wording alone can be ambiguous (a local and a global stat read the same),
// so the item's stat hashes pick among equal matches; a line that matches no
// stat exactly gets none rather than a guess.
func ListingStatID(text, modType string, hashes []string, catalog Catalog) string {
	want, wantType := normalizeStat(text), catalogType(modType)
	first := ""
	for gi := range catalog.Stats {
		for _, e := range catalog.Stats[gi].Entries {
			if e.Type != wantType || normalizeStat(e.Text) != want {
				continue
			}
			if first == "" {
				first = e.ID
			}
			if dot := strings.IndexByte(e.ID, '.'); dot >= 0 && slices.Contains(hashes, e.ID[dot+1:]) {
				return e.ID
			}
		}
	}
	return first
}

// mergeSameStats adds up affixes that roll the same stat ("96% increased
// Evasion and Energy Shield" + "41% increased Evasion and Energy Shield").
// The trade site indexes the item with the sum (137%), as the game's plain
// view shows it, so searching each part separately would match far too
// much. Lines without a stat, or whose numbers do not line up, stay apart.
func mergeSameStats(mods []ItemMod) []ItemMod {
	out := make([]ItemMod, 0, len(mods))
	at := map[string]int{}
	for _, mod := range mods {
		// An option stat without numbers (Mageblood's "Legacy of Granite" =
		// explicit.stat_264262054|6) is searched by how often the item has it:
		// GGG reads min 2 as "at least two Granites". Two separate lines would
		// each be met by a single one, so repeats fold into one counted line.
		if strings.Contains(mod.StatID, "|") && len(mod.Values) == 0 && mod.Type != "rune" {
			mod.Values = []float64{1}
		}
		i, seen := at[mod.StatID]
		if mod.StatID == "" || mod.Type == "rune" || !seen || len(out[i].Values) != len(mod.Values) || len(mod.Values) == 0 {
			if mod.StatID != "" && !seen {
				at[mod.StatID] = len(out)
			}
			out = append(out, mod)
			continue
		}
		merged := &out[i]
		if len(merged.Tiers) == 0 {
			merged.Tiers = []int{merged.Tier}
			merged.Affixes = []string{merged.Affix}
		}
		merged.Tiers = append(merged.Tiers, mod.Tier)
		merged.Affixes = append(merged.Affixes, mod.Affix)
		for vi := range merged.Values {
			merged.Values[vi] = math.Round((merged.Values[vi]+mod.Values[vi])*100) / 100
		}
		if mod.Name != "" {
			merged.Name = strings.TrimPrefix(merged.Name+" + "+mod.Name, " + ")
		}
		merged.Selected = merged.Selected || mod.Selected
		merged.Text = withValues(mod.Text, merged.Values)
	}
	return out
}

// withValues rewrites a stat line with new numbers, dropping the roll ranges
// that no longer apply to a sum.
func withValues(text string, values []float64) string {
	text = rangeRE.ReplaceAllString(text, "")
	i := 0
	return numberRE.ReplaceAllStringFunc(text, func(m string) string {
		if i >= len(values) {
			return m
		}
		v := strconv.FormatFloat(values[i], 'f', -1, 64)
		if strings.HasPrefix(m, "+") && values[i] >= 0 {
			v = "+" + v
		}
		i++
		return v
	})
}

// waystoneProperties are the totals a waystone shows above its modifiers;
// each has a trade filter of its own (map_filters).
var waystoneProperties = map[string]bool{
	"Revives Available": true, "Item Rarity": true, "Pack Size": true,
	"Monster Rarity": true, "Monster Effectiveness": true, "Waystone Drop Chance": true,
}

var requiredLevelRE = regexp.MustCompile(`\bLevel (\d+)`)
