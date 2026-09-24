package overlay

import (
	"errors"
	"math"
	"regexp"
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
	RuneSockets  int  `json:"runeSockets"`
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
}

// Snapshot is kept by the service so windows opened after the hotkey event can
// still render the latest item (or the capture error that explains why not).
type Snapshot struct {
	Item  *Item  `json:"item,omitempty"`
	Error string `json:"error,omitempty"`
}

var (
	headerRE = regexp.MustCompile(`^\{\s*(?:(Desecrated|Crafted|Fractured)\s+)?(Prefix|Suffix|Implicit|Unique|Rune|Corruption\s+Enhancement|Enhancement)(?:\s+Modifier)?(?:\s+"([^"]+)")?(?:\s+\(Tier:\s*(\d+)\))?`)
	rangeRE  = regexp.MustCompile(`\([^()]*\)`)
	numberRE = regexp.MustCompile(`[+-]?\d+(?:\.\d+)?`)
	spaceRE  = regexp.MustCompile(`\s+`)
	signedRE = regexp.MustCompile(`[+-]#`)
)

func ParseItem(raw string, catalog Catalog) (Item, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	item := Item{Raw: raw, Properties: []ItemProperty{}, Mods: []ItemMod{}}

	var title []string
	titleStart := -1
	// Weapon damage lines already include quality and local modifiers, so DPS
	// is the average hit times attacks per second (as the trade site computes).
	var physical, elemental, chaos, aps float64
	for i, source := range lines {
		line := strings.TrimSpace(source)
		plainLine := strings.TrimSpace(strings.TrimLeft(line, "# "))
		switch {
		case strings.HasPrefix(line, "Item Class:"):
			item.Class = strings.TrimSpace(strings.TrimPrefix(line, "Item Class:"))
		case strings.HasPrefix(line, "Rarity:"):
			item.Rarity = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Rarity:")))
			titleStart = i + 1
		case strings.HasPrefix(plainLine, "Item Level:"):
			item.ItemLevel = firstInt(line)
		case strings.HasPrefix(plainLine, "Requires:"):
			item.RequiredLevel = firstInt(line)
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
		case strings.HasPrefix(plainLine, "Sockets:"):
			item.RuneSockets = len(strings.Fields(strings.TrimPrefix(plainLine, "Sockets:")))
		case strings.HasPrefix(line, "Armour:") || strings.HasPrefix(line, "Evasion Rating:") || strings.HasPrefix(line, "Energy Shield:") || strings.HasPrefix(line, "Spirit:") || strings.HasPrefix(line, "Runic Ward:"):
			parts := strings.SplitN(line, ":", 2)
			item.Properties = append(item.Properties, ItemProperty{Name: parts[0], Value: strings.TrimSpace(parts[1])})
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
		for _, source := range lines[titleStart:] {
			line := strings.TrimSpace(source)
			if isSeparator(line) {
				if len(title) > 0 {
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

	var current *ItemMod
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
			matchMod(&mod, catalog)
			mod.Key = "mod-" + strconv.Itoa(len(item.Mods)+1)
			// Runes can be swapped out, so they do not describe the item's value
			// the way its own modifiers (and corruption enchants) do.
			mod.Selected = mod.StatID != "" && mod.Type != "rune"
			item.Mods = append(item.Mods, mod)
		}
		current = nil
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
			current = &ItemMod{Name: m[3]}
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
		line = strings.TrimSpace(strings.SplitN(line, " — Unscalable Value", 2)[0])
		if current.Text != "" {
			current.Text += "\n"
		}
		current.Text += line
	}
	commit()
	return item, nil
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

func matchMod(mod *ItemMod, catalog Catalog) {
	want := normalizeStat(mod.Text)
	wantType := catalogType(mod.Type)
	var fallback *StatEntry
	for gi := range catalog.Stats {
		for ei := range catalog.Stats[gi].Entries {
			e := &catalog.Stats[gi].Entries[ei]
			if wantType != "" && e.Type != wantType {
				continue
			}
			got := normalizeStat(e.Text)
			if got == want {
				mod.StatID = e.ID
				return
			}
			if fallback == nil && (strings.Contains(want, got) || strings.Contains(got, want)) {
				fallback = e
			}
		}
	}
	if fallback != nil {
		mod.StatID = fallback.ID
	}
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
