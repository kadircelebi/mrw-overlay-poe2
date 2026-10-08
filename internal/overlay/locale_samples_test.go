package overlay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Items copied from game clients in other languages (testdata/locale/<lang>),
// read against a cut-down copy of the trade site's stat catalog in that
// language (stats.local.json) and in English (stats.en.json). Each sample's
// result is kept in a .golden file next to it.
//
// POE2_LOCALE_UPDATE=<folder with trade_stats.json and trade_stats.<lang>.json>
// rebuilds the cut-down catalogs from full ones and rewrites the golden files.
func TestLocaleSamples(t *testing.T) {
	langs, _ := filepath.Glob(filepath.Join("testdata", "locale", "*"))
	if len(langs) == 0 {
		t.Fatal("no samples")
	}
	for _, dir := range langs {
		lang := filepath.Base(dir)
		t.Run(lang, func(t *testing.T) { checkLocaleSamples(t, dir, lang) })
	}
}

func checkLocaleSamples(t *testing.T, dir, lang string) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	read := func(path string) Catalog {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var r response[StatGroup]
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		return Catalog{Stats: r.Result, UpdatedAtMs: 4}
	}
	update := os.Getenv("POE2_LOCALE_UPDATE")
	if update != "" {
		local, english := read(filepath.Join(update, "trade_stats."+lang+".json")), read(filepath.Join(update, "trade_stats.json"))
		used := map[string]bool{}
		for _, f := range files {
			item := readLocaleSample(t, f, lang, local, english)
			for _, m := range item.Mods {
				used[m.StatID] = true
				for _, id := range append(m.AltStatIDs, m.WeightStats...) {
					used[id] = true
				}
			}
		}
		writeCutCatalog(t, filepath.Join(dir, "stats.local.json"), local, used)
		writeCutCatalog(t, filepath.Join(dir, "stats.en.json"), english, used)
	}
	local, english := read(filepath.Join(dir, "stats.local.json")), read(filepath.Join(dir, "stats.en.json"))
	for _, f := range files {
		got := describeSample(readLocaleSample(t, f, lang, local, english))
		golden := strings.TrimSuffix(f, ".txt") + ".golden"
		if update != "" {
			if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
			t.Errorf("%s:\n--- got\n%s--- want\n%s", filepath.Base(f), got, want)
		}
	}
}

func readLocaleSample(t *testing.T, path, lang string, local, english Catalog) Item {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if l := DetectLocale(string(raw)); l == nil || l.Lang != lang {
		t.Fatalf("%s: not read as %s", path, lang)
	}
	LocaleCatalog = func(string) (Catalog, error) { return local, nil }
	defer func() { LocaleCatalog = nil }()
	item, err := ParseItem(string(raw), english)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return item
}

func describeSample(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "lang %s | %s | %s | %q (%q) | %q (%q)\n", item.Lang, item.Class, item.Rarity,
		item.Name, item.DisplayName, item.BaseType, item.DisplayBase)
	fmt.Fprintf(&b, "ilvl %d | level %d | quality %d | rune sockets %d | fractured %v | corrupted %v\n",
		item.ItemLevel, item.RequiredLevel, item.Quality, item.RuneSockets, item.Fractured, item.Corrupted)
	fmt.Fprintf(&b, "labels: class %q | item level %q | requires %q | rare %q | corrupted %q\n", item.Labels["class"],
		item.Labels["Item Level"], item.Labels["Requires"], item.Labels["Rare"], item.Labels["Corrupted"])
	for _, m := range item.Mods {
		fmt.Fprintf(&b, "%s/%s T%d %s\n  %s\n  %s\n", m.Type, m.Affix, m.Tier, m.StatID, m.Text, m.Display)
	}
	return b.String()
}

func writeCutCatalog(t *testing.T, path string, catalog Catalog, used map[string]bool) {
	t.Helper()
	var groups []StatGroup
	for _, g := range catalog.Stats {
		cut := StatGroup{ID: g.ID, Label: g.Label}
		for _, e := range g.Entries {
			if used[e.ID] {
				cut.Entries = append(cut.Entries, e)
			}
		}
		if len(cut.Entries) > 0 {
			sort.Slice(cut.Entries, func(i, j int) bool { return cut.Entries[i].ID < cut.Entries[j].ID })
			groups = append(groups, cut)
		}
	}
	raw, err := json.MarshalIndent(response[StatGroup]{Result: groups}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
