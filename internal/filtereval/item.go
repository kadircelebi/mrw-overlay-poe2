package filtereval

import (
	"regexp"
	"strconv"
	"strings"

	"poe2filter/internal/overlay"
)

// explicitNameRE takes an explicit modifier's name from the advanced copy's
// header: { Prefix Modifier "Spirited" — Mana, Caster } or
// { Desecrated Suffix Modifier "of the Sniper" (Tier: 1) ... }.
var explicitNameRE = regexp.MustCompile(`^\{\s*(?:(?:Desecrated|Crafted|Fractured)\s+)?(?:Prefix|Suffix)\s+Modifier\s+"([^"]+)"`)

var waystoneTierRE = regexp.MustCompile(`Waystone \(Tier (\d+)\)`)

// FactsFromItem turns a parsed item (Alt+E's) into what the filter can see.
// Currency, gems and other items without an equipment rarity count as
// Normal, as in the game's filter.
func FactsFromItem(item overlay.Item) Facts {
	f := Facts{
		Class:          item.Class,
		BaseType:       item.BaseType,
		ItemLevel:      item.ItemLevel,
		Quality:        item.Quality,
		Sockets:        item.RuneSockets,
		StackSize:      item.StackSize,
		GemLevel:       item.GemLevel,
		Identified:     !item.Unidentified,
		Corrupted:      item.Corrupted || item.TwiceCorrupted,
		TwiceCorrupted: item.TwiceCorrupted,
		Mirrored:       item.Mirrored,
		Fractured:      item.Fractured,
	}
	if f.BaseType == "" {
		f.BaseType = item.Name
	}
	switch strings.ToLower(item.Rarity) {
	case "magic":
		f.Rarity = "Magic"
	case "rare":
		f.Rarity = "Rare"
	case "unique":
		f.Rarity = "Unique"
	default:
		f.Rarity = "Normal"
	}
	if item.GemSockets > 0 {
		f.Sockets = item.GemSockets
	}
	if m := waystoneTierRE.FindStringSubmatch(f.BaseType); m != nil {
		f.WaystoneTier, _ = strconv.Atoi(m[1])
	}
	for _, p := range item.Properties {
		if strings.EqualFold(p.Name, "Waystone Tier") && f.WaystoneTier == 0 {
			f.WaystoneTier, _ = strconv.Atoi(strings.TrimSpace(p.Value))
		}
	}
	for _, m := range item.Mods {
		if m.Type == "enchant" {
			f.AnyEnchantment = true
		}
	}
	for _, line := range strings.Split(item.Raw, "\n") {
		if m := explicitNameRE.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			f.ExplicitMods = append(f.ExplicitMods, m[1])
		}
	}
	return f
}
