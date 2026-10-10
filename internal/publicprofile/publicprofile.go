// Package publicprofile is the shape of a filter profile shared through the
// profile server. The app and the server use the same code, so a document
// the server stores is exactly one the app accepts, and the app still checks
// every download itself.
//
// Only the settings that shape the filter travel: the threshold, the rules,
// the player's groups (with their hidden items and Exotic changes), the looks
// and sounds, and the lists. League, language, filter name, file paths, price
// sources, scanning and schedule stay on each player's own machine.
package publicprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"poe2filter/internal/filter"
)

// FormatVersion is the version of Document. A document of another version is
// refused rather than guessed at.
const FormatVersion = 1

// Limits of a document. MaxBytes bounds what is read at all; the rest bound
// what a valid document may hold.
const (
	MaxBytes       = 256 << 10
	MaxNameLen     = 40
	MaxDescLen     = 300
	MaxTags        = 5
	MaxTextLen     = 120 // a list entry, a base or a modifier name
	MaxGroupName   = 60
	MaxListLen     = 2000
	MaxExoticItems = 1000
	MaxMapEntries  = 200
	MaxValue       = 1_000_000 // a price threshold, in any unit
)

// Tags are the labels an author may put on a profile, as fixed ids; the app
// shows them translated. Nobody types a tag of their own.
var Tags = []string{
	"general", "leveling", "mapping", "expedition", "abyss", "delirium",
	"breach", "ritual", "essence", "bosses", "ssf", "strict",
}

// Document is what is published and downloaded.
type Document struct {
	Format int    `json:"format"`
	Filter Filter `json:"filter"`
}

// Filter holds the shared settings, with the same JSON names as filter.Config.
type Filter struct {
	// Threshold
	MinValue     float64 `json:"min_value"`
	MinValueUnit string  `json:"min_value_unit"`
	FilterMode   string  `json:"filter_mode"`
	Strictness   int     `json:"strictness"`

	// Rules
	IncludeGear       bool `json:"include_gear"`
	T5RareTier        int  `json:"t5_rare_tier"`
	RareJewelTier     int  `json:"rare_jewel_tier"`
	QualityThreshold  int  `json:"quality_threshold"`
	WaystoneTier      int  `json:"waystone_tier"`
	UncutGemLevel     int  `json:"uncut_gem_level"`
	UncutSupportLevel int  `json:"uncut_support_level"`
	PinnacleKeys      bool `json:"boss_keys_and_tablets"`
	HideExalt         bool `json:"hide_exalt"`
	HideGold          bool `json:"hide_gold"`

	// Groups
	ItemGroups  []filter.ItemGroup    `json:"item_groups"`
	Exotic      filter.ExoticSettings `json:"exotic"`
	ShowExotics bool                  `json:"show_exotics"`
	HiddenItems []filter.HiddenItem   `json:"hidden_items"`
	HiddenOff   bool                  `json:"hidden_off"`

	// Look and sounds
	Styles       map[string]string             `json:"styles"`
	CustomStyles map[string]filter.CustomStyle `json:"custom_styles"`
	Sounds       map[string]string             `json:"sounds"`
	FontSizes    map[string]int                `json:"font_sizes"`
	Volumes      map[string]int                `json:"volumes"`

	// Lists
	Whitelist   []string `json:"whitelist"`
	ChanceBases []string `json:"chance_bases"`
}

// Meta describes a published profile in the listing.
type Meta struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// Listing is a published profile as the server lists it.
type Listing struct {
	ID          string   `json:"id"`
	Author      string   `json:"author"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Version     int      `json:"version"`
	Followers   int      `json:"followers"`
	UpdatedAt   int64    `json:"updated_at"`
	Hidden      bool     `json:"hidden,omitempty"`
}

// SoundSwap is a style group whose own sound file was left out of the shared
// profile; followers hear the group's built-in sound instead.
type SoundSwap struct {
	Group string `json:"group"` // style group key, as in filter.Config.Sounds
	File  string `json:"file"`  // the file name the author uses
}

// FromConfig takes the shared settings out of cfg. Sound files are not
// shared (they live on the author's disk), so groups using one fall back to
// their built-in sound and are reported.
func FromConfig(cfg filter.Config) (Document, []SoundSwap) {
	cfg.Normalize()
	f := Filter{
		MinValue: cfg.MinValue, MinValueUnit: cfg.MinValueUnit, FilterMode: cfg.FilterMode, Strictness: cfg.Strictness,
		IncludeGear: cfg.IncludeGear, T5RareTier: cfg.T5RareTier, RareJewelTier: cfg.RareJewelTier,
		QualityThreshold: cfg.QualityThreshold, WaystoneTier: cfg.WaystoneTier, UncutGemLevel: cfg.UncutGemLevel,
		UncutSupportLevel: cfg.UncutSupportLevel, PinnacleKeys: cfg.PinnacleKeys, HideExalt: cfg.HideExalt, HideGold: cfg.HideGold,
		ItemGroups: cfg.ItemGroups, Exotic: cfg.Exotic, ShowExotics: cfg.ShowExotics,
		HiddenItems: cfg.HiddenItems, HiddenOff: cfg.HiddenOff,
		Styles: cfg.Styles, CustomStyles: cfg.CustomStyles, Sounds: map[string]string{},
		FontSizes: cfg.FontSizes, Volumes: cfg.Volumes,
		Whitelist: cfg.Whitelist, ChanceBases: cfg.ChanceBases,
	}
	var swaps []SoundSwap
	for group, v := range cfg.Sounds {
		if file, ok := strings.CutPrefix(v, filter.SoundFilePrefix); ok {
			swaps = append(swaps, SoundSwap{Group: group, File: file})
			continue
		}
		f.Sounds[group] = v
	}
	sortSwaps(swaps)
	doc := Document{Format: FormatVersion, Filter: f}
	doc.Filter = clean(doc.Filter)
	return doc, swaps
}

// Apply puts the shared settings over local, keeping the player's own
// league, language, filter name, paths and scanning, and returns the result.
func (d Document) Apply(local filter.Config) filter.Config {
	c := applyRaw(clean(d.Filter), local)
	// A document carries no settings version, and the strict format leaves no
	// room for one. The waystone slider's old default now hides T1-T13, so a
	// shared 14 is read as "NeverSink decides": at worst a follower sees a few
	// more waystones, never fewer.
	if c.WaystoneTier == filter.LegacyWaystoneDefault {
		c.WaystoneTier = filter.TierOff
	}
	c.Normalize()
	return c
}

// Decode reads a document strictly: size-limited, no unknown fields, nothing
// after it, every value within its limits. The result is normalised the way
// the app would normalise its own settings.
func Decode(data []byte) (Document, error) {
	if len(data) > MaxBytes {
		return Document{}, fmt.Errorf("document larger than %d bytes", MaxBytes)
	}
	var d Document
	if err := decodeStrict(data, &d); err != nil {
		return Document{}, err
	}
	if d.Format != FormatVersion {
		return Document{}, fmt.Errorf("unsupported format %d", d.Format)
	}
	if err := d.Filter.validate(); err != nil {
		return Document{}, err
	}
	d.Filter = clean(d.Filter)
	return d, nil
}

// Encode writes the canonical form of d: what the server stores and serves
// is always re-encoded from the parsed values, never the bytes it received.
func Encode(d Document) ([]byte, error) {
	if err := d.Filter.validate(); err != nil {
		return nil, err
	}
	d.Format = FormatVersion
	d.Filter = clean(d.Filter)
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("document larger than %d bytes", MaxBytes)
	}
	return data, nil
}

// DecodeMeta reads a listing entry as strictly as Decode.
func DecodeMeta(data []byte) (Meta, error) {
	if len(data) > 4<<10 {
		return Meta{}, errors.New("meta too large")
	}
	var m Meta
	if err := decodeStrict(data, &m); err != nil {
		return Meta{}, err
	}
	return m.Clean()
}

// Clean checks m and returns it trimmed, with tags deduplicated in the
// fixed order.
func (m Meta) Clean() (Meta, error) {
	m.Name = strings.TrimSpace(m.Name)
	m.Description = strings.TrimSpace(m.Description)
	if m.Name == "" {
		return Meta{}, errors.New("name is empty")
	}
	if err := checkText("name", m.Name, MaxNameLen); err != nil {
		return Meta{}, err
	}
	if err := checkText("description", m.Description, MaxDescLen); err != nil {
		return Meta{}, err
	}
	want := map[string]bool{}
	for _, t := range m.Tags {
		if !isTag(t) {
			return Meta{}, fmt.Errorf("unknown tag %q", t)
		}
		want[t] = true
	}
	if len(want) > MaxTags {
		return Meta{}, fmt.Errorf("more than %d tags", MaxTags)
	}
	m.Tags = []string{}
	for _, t := range Tags {
		if want[t] {
			m.Tags = append(m.Tags, t)
		}
	}
	return m, nil
}

func isTag(t string) bool {
	for _, x := range Tags {
		if t == x {
			return true
		}
	}
	return false
}

func decodeStrict(data []byte, v any) error {
	if !utf8.Valid(data) {
		return errors.New("not UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("data after the document")
	}
	return nil
}

// clean runs the app's own normalisation over f, so shared settings obey the
// same rules as local ones (known style groups, valid sounds and colours,
// tier ranges, list limits...).
func clean(f Filter) Filter {
	c := applyRaw(f, filter.DefaultConfig())
	c.Normalize()
	for group, v := range c.Sounds {
		if strings.HasPrefix(v, filter.SoundFilePrefix) {
			delete(c.Sounds, group)
		}
	}
	return Filter{
		MinValue: c.MinValue, MinValueUnit: c.MinValueUnit, FilterMode: c.FilterMode, Strictness: c.Strictness,
		IncludeGear: c.IncludeGear, T5RareTier: c.T5RareTier, RareJewelTier: c.RareJewelTier,
		QualityThreshold: c.QualityThreshold, WaystoneTier: c.WaystoneTier, UncutGemLevel: c.UncutGemLevel,
		UncutSupportLevel: c.UncutSupportLevel, PinnacleKeys: c.PinnacleKeys, HideExalt: c.HideExalt, HideGold: c.HideGold,
		ItemGroups: c.ItemGroups, Exotic: c.Exotic, ShowExotics: c.ShowExotics,
		HiddenItems: c.HiddenItems, HiddenOff: c.HiddenOff,
		Styles: c.Styles, CustomStyles: c.CustomStyles, Sounds: c.Sounds,
		FontSizes: c.FontSizes, Volumes: c.Volumes,
		Whitelist: c.Whitelist, ChanceBases: c.ChanceBases,
	}
}

// applyRaw copies f into c without normalising; clean and Apply build on it.
func applyRaw(f Filter, c filter.Config) filter.Config {
	c.MinValue, c.MinValueUnit, c.FilterMode, c.Strictness = f.MinValue, f.MinValueUnit, f.FilterMode, f.Strictness
	c.IncludeGear, c.T5RareTier, c.RareJewelTier = f.IncludeGear, f.T5RareTier, f.RareJewelTier
	c.QualityThreshold, c.WaystoneTier, c.UncutGemLevel = f.QualityThreshold, f.WaystoneTier, f.UncutGemLevel
	c.UncutSupportLevel, c.PinnacleKeys, c.HideExalt, c.HideGold = f.UncutSupportLevel, f.PinnacleKeys, f.HideExalt, f.HideGold
	c.ItemGroups, c.Exotic, c.ShowExotics = cloneGroups(f.ItemGroups), cloneExotic(f.Exotic), f.ShowExotics
	c.HiddenItems, c.HiddenOff = append([]filter.HiddenItem(nil), f.HiddenItems...), f.HiddenOff
	c.Styles, c.CustomStyles, c.Sounds = cloneMap(f.Styles), cloneMap(f.CustomStyles), cloneMap(f.Sounds)
	c.FontSizes, c.Volumes = cloneMap(f.FontSizes), cloneMap(f.Volumes)
	c.Whitelist, c.ChanceBases = append([]string(nil), f.Whitelist...), append([]string(nil), f.ChanceBases...)
	c.DivineTheme, c.DivineSound = "", ""
	return c
}

// validate refuses what normalisation would otherwise quietly repair: text
// with control characters, oversized lists, numbers out of range. A document
// that fails was not written by the app.
func (f Filter) validate() error {
	if err := checkNumber("min_value", f.MinValue); err != nil {
		return err
	}
	if len(f.ItemGroups) > filter.MaxItemGroups {
		return fmt.Errorf("more than %d groups", filter.MaxItemGroups)
	}
	for i, g := range f.ItemGroups {
		where := fmt.Sprintf("item_groups[%d]", i)
		if err := checkText(where+".id", g.ID, 20); err != nil {
			return err
		}
		if err := checkText(where+".name", g.Name, MaxGroupName); err != nil {
			return err
		}
		if err := checkNumber(where+".threshold_value", g.ThresholdValue); err != nil {
			return err
		}
		if err := checkList(where+".items", g.Items, MaxListLen); err != nil {
			return err
		}
	}
	if len(f.HiddenItems) > filter.MaxHiddenItems {
		return fmt.Errorf("more than %d hidden items", filter.MaxHiddenItems)
	}
	for i, h := range f.HiddenItems {
		if err := checkText(fmt.Sprintf("hidden_items[%d].base", i), h.Base, MaxTextLen); err != nil {
			return err
		}
		if len(h.Rarities) > 4 {
			return errors.New("hidden item with too many rarities")
		}
	}
	x := f.Exotic
	if len(x.Added) > MaxExoticItems || len(x.Removed) > MaxExoticItems || len(x.Levels) > MaxExoticItems {
		return errors.New("exotic changes too large")
	}
	for i, e := range x.Added {
		where := fmt.Sprintf("exotic.added[%d]", i)
		for _, s := range []string{e.Key, e.Kind, e.Base, e.Stat, e.Label, e.Level, e.Source, e.Tier} {
			if err := checkText(where, s, 2*MaxTextLen); err != nil {
				return err
			}
		}
		for _, l := range [][]string{e.Classes, e.Names, e.Conds} {
			if err := checkList(where, l, 100); err != nil {
				return err
			}
		}
	}
	if err := checkList("exotic.removed", x.Removed, MaxExoticItems); err != nil {
		return err
	}
	for k, v := range x.Levels {
		if err := checkText("exotic.levels", k, 2*MaxTextLen); err != nil {
			return err
		}
		if err := checkText("exotic.levels", v, 20); err != nil {
			return err
		}
	}
	for name, n := range map[string]int{"styles": len(f.Styles), "custom_styles": len(f.CustomStyles),
		"sounds": len(f.Sounds), "font_sizes": len(f.FontSizes), "volumes": len(f.Volumes)} {
		if n > MaxMapEntries {
			return fmt.Errorf("%s has more than %d entries", name, MaxMapEntries)
		}
	}
	for _, m := range []map[string]string{f.Styles, f.Sounds} {
		for k, v := range m {
			if err := checkText("style key", k, 40); err != nil {
				return err
			}
			if err := checkText("style value", v, MaxTextLen); err != nil {
				return err
			}
		}
	}
	for k, s := range f.CustomStyles {
		if err := checkText("custom style key", k, 40); err != nil {
			return err
		}
		for _, v := range []string{s.Bg, s.Text, s.Border, s.Beam, s.Icon, s.Shape} {
			if err := checkText("custom style", v, 20); err != nil {
				return err
			}
		}
	}
	for _, m := range []map[string]int{f.FontSizes, f.Volumes} {
		for k := range m {
			if err := checkText("style key", k, 40); err != nil {
				return err
			}
		}
	}
	if err := checkList("whitelist", f.Whitelist, MaxListLen); err != nil {
		return err
	}
	return checkList("chance_bases", f.ChanceBases, MaxListLen)
}

func checkNumber(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > MaxValue {
		return fmt.Errorf("%s out of range", name)
	}
	return nil
}

func checkList(name string, list []string, max int) error {
	if len(list) > max {
		return fmt.Errorf("%s has more than %d entries", name, max)
	}
	for _, s := range list {
		if err := checkText(name, s, MaxTextLen); err != nil {
			return err
		}
	}
	return nil
}

// checkText refuses invalid UTF-8, control characters (line breaks
// included) and text longer than max characters.
func checkText(name, s string, max int) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not UTF-8", name)
	}
	if utf8.RuneCountInString(s) > max {
		return fmt.Errorf("%s longer than %d characters", name, max)
	}
	if filter.CleanText(s) != strings.TrimSpace(s) {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}

func cloneMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneGroups(in []filter.ItemGroup) []filter.ItemGroup {
	out := make([]filter.ItemGroup, len(in))
	for i, g := range in {
		g.Items = append([]string(nil), g.Items...)
		out[i] = g
	}
	return out
}

func cloneExotic(x filter.ExoticSettings) filter.ExoticSettings {
	out := filter.ExoticSettings{
		Added:   make([]filter.ExoticEntry, len(x.Added)),
		Removed: append([]string(nil), x.Removed...),
		Levels:  cloneMap(x.Levels),
	}
	for i, e := range x.Added {
		e.Classes, e.Names, e.Conds = append([]string(nil), e.Classes...), append([]string(nil), e.Names...), nil
		out.Added[i] = e
	}
	return out
}

func sortSwaps(s []SoundSwap) {
	sort.Slice(s, func(i, j int) bool { return s[i].Group < s[j].Group })
}
