package main

import (
	"context"
	"sort"
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
	texts := map[string]string{}
	for _, g := range catalog.Stats {
		for _, e := range g.Entries {
			texts[e.ID] = e.Text
		}
	}
	tables := data.For("", class)
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
	for _, t := range tables {
		opt := ExoticModOption{Stat: t.Stat, Text: label(t), Affix: t.Affix}
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
	return out, nil
}

func appendUnique(list []string, v string) []string {
	for _, have := range list {
		if have == v {
			return list
		}
	}
	return append(list, v)
}
