package overlay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryLanguageReadsARareItem writes a rare helmet with each language's
// own words and the trade site's catalog wording in that language, then reads
// it back. It checks the tables and the catalogs fit together for languages
// no copied sample exists for yet; the copy's real layout is the samples'
// job (TestLocaleSamples). POE2_LOCALE_STATS names a folder with
// trade_stats.json and trade_stats.<lang>.json.
func TestEveryLanguageReadsARareItem(t *testing.T) {
	dir := os.Getenv("POE2_LOCALE_STATS")
	if dir == "" {
		t.Skip("POE2_LOCALE_STATS not set")
	}
	read := func(path string) Catalog {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var r response[StatGroup]
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		return Catalog{Stats: r.Result, UpdatedAtMs: 5}
	}
	english := read(filepath.Join(dir, "trade_stats.json"))
	tierRE := regexp.MustCompile(`\\\((.+?)\(\?P<tier>`)
	for _, l := range Locales() {
		t.Run(l.Lang, func(t *testing.T) {
			local := read(filepath.Join(dir, "trade_stats."+l.Lang+".json"))
			wording := func(id, value string) string {
				for _, g := range local.Stats {
					for _, e := range g.Entries {
						if e.ID == id {
							return strings.Replace(e.Text, "#", value, 1)
						}
					}
				}
				t.Fatalf("%s not in the catalog", id)
				return ""
			}
			tierWord := "Tier: "
			if p := l.patterns["MODIFIER_LINE"]; p != nil {
				if m := tierRE.FindStringSubmatch(p.String()); m != nil {
					tierWord = strings.ReplaceAll(m[1], `\`, "")
				}
			}
			open, close := `"`, `"`
			if strings.Contains(l.patterns["MODIFIER_LINE"].String(), "«") {
				open, close = "« ", " »"
			}
			word := func(k string) string { return strings.Trim(l.local[k], ` :«"`) }
			raw := strings.Join([]string{
				l.local["ITEM_CLASS"] + "x",
				l.local["RARITY"] + l.local["RARITY_RARE"],
				"Doom Crown",
				localItemName(t, l, "Kamasan Tiara"),
				"--------",
				l.local["ITEM_LEVEL"] + "81",
				"--------",
				"{ " + word("PREFIX_MODIFIER") + " " + open + "a" + close + " (" + tierWord + "1) }",
				wording("explicit.stat_3299347043", "142"),
				"{ " + word("CRAFTED_MODIFIER") + " " + word("SUFFIX_MODIFIER") + " " + open + "b" + close + " (" + tierWord + "2) }",
				wording("explicit.stat_4220027924", "39"),
				"--------",
				strings.TrimSpace(l.local["CORRUPTED"]),
			}, "\n")
			LocaleCatalog = func(string) (Catalog, error) { return local, nil }
			defer func() { LocaleCatalog = nil }()
			item, err := ParseItem(raw, english)
			if err != nil {
				t.Fatal(err)
			}
			mods := ownMods(item.Mods)
			if item.Lang != l.Lang || item.BaseType != "Kamasan Tiara" || item.Class != "Helmets" || item.ItemLevel != 81 ||
				!item.Corrupted || len(mods) != 2 ||
				mods[0].StatID != "explicit.stat_3299347043" || mods[0].Tier != 1 || mods[0].Affix != "prefix" ||
				mods[1].StatID != "crafted.stat_4220027924" || mods[1].Tier != 2 || mods[1].Affix != "suffix" {
				t.Errorf("read %+v %q %q ilvl %d corrupted %v\nmods %+v\n%s", item.Lang, item.BaseType, item.Class,
					item.ItemLevel, item.Corrupted, mods, raw)
			}
		})
	}
}
