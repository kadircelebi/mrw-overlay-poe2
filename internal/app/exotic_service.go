package app

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"poe2filter/internal/filter"
	"poe2filter/internal/overlay"
)

// ExoticNeverSink lists NeverSink's exotic entries (bases and modifier
// names) from the base filter on disk; the panel lays the player's changes
// (Config.Exotic) over them.
func (s *AppService) ExoticNeverSink() []filter.ExoticEntry {
	return filter.NeverSinkExotics(s.eng.NeverSinkExotics())
}

// ExoticTier is one tier of a modifier: its name is what the filter matches.
type ExoticTier struct {
	Tier  int     `json:"tier"`
	Name  string  `json:"name"`
	Level int     `json:"level"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	// Also are other modifiers of the class that use the same name; an
	// entry with this tier catches them too.
	Also []string `json:"also,omitempty"`
}

// ExoticModOption is a modifier an item class can roll, for the Exotic
// group's "add a modifier" search.
type ExoticModOption struct {
	Stat  string       `json:"stat"`
	Text  string       `json:"text"`
	Affix string       `json:"affix"`
	Tiers []ExoticTier `json:"tiers"`
	// Pool names the special pool of a modifier outside the normal drop
	// pool ("genesis_tree_caster": Breach's genesis tree); the export does
	// not say which item classes it reaches.
	Pool string `json:"pool,omitempty"`
}

// ExoticModOptions lists the explicit modifiers items of the class can roll,
// with their tiers (best first) and the names they share with other
// modifiers of the class.
func (s *AppService) ExoticModOptions(class string) ([]ExoticModOption, error) {
	data, err := s.overlayTiers.Load(context.Background())
	if err != nil {
		return nil, err
	}
	catalog, _ := s.overlayCatalog.Load(context.Background())
	return exoticModOptions(data, catalog, class), nil
}

func exoticModOptions(data *overlay.TierData, catalog overlay.Catalog, class string) []ExoticModOption {
	texts := map[string]string{}
	for _, g := range catalog.Stats {
		for _, e := range g.Entries {
			texts[e.ID] = e.Text
		}
	}
	tables := data.For("", class)
	pools := make([]string, len(tables))
	for _, pool := range data.PoolNames() {
		for _, t := range data.Pool(pool) {
			tables = append(tables, t)
			pools = append(pools, pool)
		}
	}
	label := func(t overlay.TierTable) string {
		text := texts[t.Stat]
		if text == "" {
			text = t.Stat
		}
		if t.With != "" {
			text += " + " + t.With
		}
		return text
	}
	users := map[string][]string{} // tier name -> modifiers using it
	for _, t := range tables {
		for _, tier := range t.Tiers {
			users[tier.Name] = appendUnique(users[tier.Name], label(t))
		}
	}
	out := make([]ExoticModOption, 0, len(tables))
	for ti, t := range tables {
		if len(t.Tiers) == 0 {
			continue // Exotic rules name tiers; a line without a value has none
		}
		opt := ExoticModOption{Stat: t.Stat, Text: label(t), Affix: t.Affix, Pool: pools[ti]}
		for _, tier := range t.Tiers {
			et := ExoticTier{Tier: tier.Tier, Name: tier.Name, Level: tier.Level, Min: tier.Min, Max: tier.Max}
			for _, other := range users[tier.Name] {
				if other != opt.Text {
					et.Also = append(et.Also, other)
				}
			}
			opt.Tiers = append(opt.Tiers, et)
		}
		sort.Slice(opt.Tiers, func(i, j int) bool { return opt.Tiers[i].Tier < opt.Tiers[j].Tier })
		out = append(out, opt)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Text) < strings.ToLower(out[j].Text) })
	return out
}

// modifierOption finds the option a copied modifier belongs to: the one
// with a tier of that name, and when several share the name, the one whose
// text reads like the modifier's stat line. ok is false when none has it.
func modifierOption(options []ExoticModOption, name, line string) (ExoticModOption, int, bool) {
	var found []ExoticModOption
	var tiers []int
	for _, o := range options {
		for _, t := range o.Tiers {
			if strings.EqualFold(t.Name, name) {
				found = append(found, o)
				tiers = append(tiers, t.Tier)
				break
			}
		}
	}
	if len(found) == 0 {
		return ExoticModOption{}, 0, false
	}
	for i, o := range found {
		if statTextMatches(o.Text, line) {
			return o, tiers[i], true
		}
	}
	return found[0], tiers[0], true
}

var statNumberRE = `[+-]?\d+(?:\.\d+)?`

// statTextMatches says whether a stat line ("19% increased Cast Speed")
// reads as the option's text ("#% increased Cast Speed"); for a hybrid
// option only its first stat is compared.
func statTextMatches(text, line string) bool {
	text, _, _ = strings.Cut(text, " + ")
	if text == "" || line == "" {
		return false
	}
	pattern := strings.ReplaceAll(regexp.QuoteMeta(text), "#", statNumberRE)
	re, err := regexp.Compile(`(?i)^` + pattern + `$`)
	return err == nil && re.MatchString(strings.TrimSpace(line))
}

// exoticModLabel is a modifier entry's label as the game prints it, with
// the lowest value kept: "19+% increased Cast Speed".
func exoticModLabel(text string, low ExoticTier) string {
	if !strings.Contains(text, "#") {
		return text
	}
	v := strconv.FormatFloat(low.Min, 'f', -1, 64)
	return strings.Replace(text, "#", v+"+", 1)
}

func appendUnique(list []string, v string) []string {
	for _, have := range list {
		if have == v {
			return list
		}
	}
	return append(list, v)
}

// ExoticCandidate is something of a copied item that can go to the Exotic
// group: its base, or one of its named explicit modifiers.
type ExoticCandidate struct {
	Entry filter.ExoticEntry `json:"entry"`
	// Present is set when the group already shows it.
	Present bool `json:"present"`
	// Tiers are a modifier's tiers (best first) when the tier data knows
	// it, Tier the copied item's; Entry then keeps that tier and better.
	// Covered are the tier names the group already shows for the class;
	// Text is the modifier's text with "#" for the value.
	Text    string       `json:"text,omitempty"`
	Tiers   []ExoticTier `json:"tiers,omitempty"`
	Tier    int          `json:"tier,omitempty"`
	Covered []string     `json:"covered,omitempty"`
}

// ExoticCandidates lists the copied item's base and named explicit
// modifiers (as the advanced copy writes them) for Alt+E's "add to Exotic".
// Only Normal, Magic and Rare gear qualifies.
func (s *AppService) ExoticCandidates(raw string) ([]ExoticCandidate, error) {
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		return nil, err
	}
	item, err := overlay.ParseItem(raw, catalog)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(item.Rarity) {
	case "normal", "magic", "rare":
	default:
		return []ExoticCandidate{}, nil
	}
	if item.StackSize > 0 || item.Exchange != "" || item.Class == "" {
		return []ExoticCandidate{}, nil
	}
	cfg := s.eng.Config()
	present := map[string]bool{}
	for _, r := range filter.ExoticRows(filter.NeverSinkExotics(s.eng.NeverSinkExotics()), cfg.Exotic) {
		if r.Off {
			continue
		}
		if r.Kind == filter.ExoticBase {
			present["base|"+strings.ToLower(r.Base)] = true
			continue
		}
		for _, c := range r.Classes {
			for _, n := range r.Names {
				present["mod|"+c+"|"+strings.ToLower(n)] = true
			}
		}
	}
	out := []ExoticCandidate{{
		Entry:   filter.ExoticEntry{Kind: filter.ExoticBase, Base: item.BaseType, Level: filter.ExoticNormal, Source: "user"},
		Present: present["base|"+strings.ToLower(item.BaseType)],
	}}
	// Without tier data a modifier is offered by its own name only.
	var options []ExoticModOption
	if data, err := s.overlayTiers.Load(context.Background()); err == nil {
		options = exoticModOptions(data, catalog, item.Class)
	}
	for _, m := range namedModifiers(raw) {
		e := filter.ExoticEntry{Kind: filter.ExoticMod, Classes: []string{item.Class}, Names: []string{m.name},
			Label: m.text + ` ("` + m.name + `")`, Level: filter.ExoticNormal, Source: "user"}
		c := ExoticCandidate{Entry: e, Present: present["mod|"+item.Class+"|"+strings.ToLower(m.name)]}
		if o, tier, ok := modifierOption(options, m.name, m.text); ok {
			var kept []ExoticTier
			for _, t := range o.Tiers {
				if t.Tier <= tier {
					kept = append(kept, t)
				}
				if present["mod|"+item.Class+"|"+strings.ToLower(t.Name)] {
					c.Covered = append(c.Covered, t.Name)
				}
			}
			c.Entry.Names = nil
			for _, t := range kept {
				c.Entry.Names = append(c.Entry.Names, t.Name)
			}
			c.Entry.Stat, c.Entry.MinTier = o.Stat, tier
			c.Entry.Label = exoticModLabel(o.Text, kept[len(kept)-1])
			c.Text, c.Tiers, c.Tier = o.Text, o.Tiers, tier
			c.Present = len(c.Covered) > 0 && coversAll(c.Covered, c.Entry.Names)
		}
		out = append(out, c)
	}
	return out, nil
}

// coversAll says whether every name is among have.
func coversAll(have, names []string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[strings.ToLower(h)] = true
	}
	for _, n := range names {
		if !set[strings.ToLower(n)] {
			return false
		}
	}
	return true
}

// AddExotic adds an entry to the Exotic group and rewrites the filter.
func (s *AppService) AddExotic(entry filter.ExoticEntry) (HideResult, error) {
	cfg := s.eng.Config()
	cfg.Exotic.Added = append(replacedExotic(cfg.Exotic.Added, entry), entry)
	cfg.ShowExotics = true
	saved, err := s.SaveConfig(cfg)
	if err != nil {
		return HideResult{}, err
	}
	return HideResult{Hidden: saved.HiddenItems, Updating: s.eng.UpdateNow() == nil}, nil
}

// replacedExotic drops the player's entries a modifier added again (from
// Alt+E with other tiers) replaces: same class and stat, and a tier name in
// common. The name matters: a prefix and a suffix can share the stat
// ("Hoarder's" and "of Archaeology", both item rarity).
func replacedExotic(added []filter.ExoticEntry, entry filter.ExoticEntry) []filter.ExoticEntry {
	if entry.Kind != filter.ExoticMod || entry.Stat == "" {
		return added
	}
	kept := added[:0:0]
	for _, e := range added {
		if e.Kind == filter.ExoticMod && e.Stat == entry.Stat &&
			strings.Join(e.Classes, ",") == strings.Join(entry.Classes, ",") && sharesName(e.Names, entry.Names) {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

func sharesName(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}

type namedModifier struct{ name, text string }

// rangeRE is a roll's range after its value: "29(26-32)%".
var rangeRE = regexp.MustCompile(`\(-?[\d.]+-?-?[\d.]*\)`)

var explicitHeaderRE = regexp.MustCompile(`^\{\s*(?:(?:Desecrated|Crafted|Fractured)\s+)?(?:Prefix|Suffix)\s+Modifier\s+"([^"]+)"`)

// namedModifiers takes each explicit modifier's name and first stat line
// from the advanced copy's text.
func namedModifiers(raw string) []namedModifier {
	var out []namedModifier
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i, line := range lines {
		m := explicitHeaderRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		text := ""
		for _, next := range lines[i+1:] {
			next = strings.TrimSpace(next)
			if next == "" || strings.HasPrefix(next, "{") || strings.HasPrefix(next, "---") {
				break
			}
			if !strings.HasPrefix(next, "(") {
				text = next
				break
			}
		}
		out = append(out, namedModifier{name: m[1], text: strings.TrimSpace(rangeRE.ReplaceAllString(text, ""))})
	}
	return out
}
