package overlay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// localItemName is the German name of an English base, from the tables.
func localItemName(t *testing.T, l *Locale, english string) string {
	t.Helper()
	for local, n := range l.items {
		if n.en == english {
			return l.itemNames[local]
		}
	}
	t.Fatalf("no German name for %s", english)
	return ""
}

func germanTestCatalogs() (english, german Catalog) {
	english = testCatalog()
	english.Stats[0].Entries = append(english.Stats[0].Entries,
		StatEntry{ID: "explicit.hybrid", Text: "#% increased Energy Shield\n# to maximum Life", Type: "explicit"},
		StatEntry{ID: "explicit.cold", Text: "#% to Cold Resistance", Type: "explicit"},
	)
	german = Catalog{UpdatedAtMs: 1, Stats: []StatGroup{{Label: "Explizit", Entries: []StatEntry{
		{ID: "explicit.hybrid", Text: "#% erhöhter Energieschild\n# zu maximalem Leben", Type: "explicit"},
		{ID: "explicit.cold", Text: "#% zu Kälteresistenz", Type: "explicit"},
	}}}}
	return english, german
}

func TestGermanRareReadsAsTheEnglishItem(t *testing.T) {
	de := LocaleByLang("de")
	if de == nil {
		t.Fatal("no German tables embedded")
	}
	base := localItemName(t, de, "Kamasan Tiara")
	raw := "Gegenstandsklasse: Helme\n" +
		"Seltenheit: Selten\n" +
		"Rache-Stern\n" +
		base + "\n" +
		"--------\n" +
		"Qualität (Verteidigungsmodifikatoren): +40% (augmentiert)\n" +
		"Energieschild: 503 (augmentiert)\n" +
		"## Erfordert: Stufe 75, 103 (augmentiert) Int\n" +
		"## Gegenstandsstufe: 81\n" +
		`{ Präfix-Modifikator "Papst" (Level: 1) — Leben, Energieschild }` + "\n" +
		"41(39-42)% erhöhter Energieschild\n" +
		"+44(42-49) zu maximalem Leben\n" +
		`{ Suffix-Modifikator "des Eises" (Level: 2) — Elementar, Kälte, Resistenz }` + "\n" +
		"+39(36-40)% zu Kälteresistenz\n" +
		"--------\n" +
		"Verderbt"
	english, german := germanTestCatalogs()
	LocaleCatalog = func(lang string) (Catalog, error) { return german, nil }
	defer func() { LocaleCatalog = nil }()

	item, err := ParseItem(raw, english)
	if err != nil {
		t.Fatal(err)
	}
	if item.Lang != "de" || item.BaseType != "Kamasan Tiara" || item.Class != "Helmets" || item.Rarity != "rare" {
		t.Fatalf("item: lang %q base %q class %q rarity %q\n%s", item.Lang, item.BaseType, item.Class, item.Rarity, item.Raw)
	}
	if item.ItemLevel != 81 || item.Quality != 40 || !item.Corrupted {
		t.Fatalf("header: ilvl %d quality %d corrupted %v\n%s", item.ItemLevel, item.Quality, item.Corrupted, item.Raw)
	}
	mods := ownMods(item.Mods)
	if len(mods) != 2 || mods[0].StatID != "explicit.hybrid" || mods[0].Tier != 1 || mods[0].Affix != "prefix" ||
		mods[1].StatID != "explicit.cold" || mods[1].Tier != 2 || mods[1].Affix != "suffix" {
		t.Fatalf("mods: %+v\n%s", mods, item.Raw)
	}
	if v := mods[0].Values; len(v) != 2 || v[0] != 41 || v[1] != 44 {
		t.Fatalf("values: %v", v)
	}
	// Shown as the German game writes it.
	if item.DisplayBase != base || item.DisplayName != "" ||
		mods[0].Display != "41(39-42)% erhöhter Energieschild\n+44(42-49) zu maximalem Leben" ||
		mods[1].Display != "+39(36-40)% zu Kälteresistenz" {
		t.Fatalf("display: base %q name %q mods %q / %q", item.DisplayBase, item.DisplayName, mods[0].Display, mods[1].Display)
	}
}

func TestEnglishTextIsNotTranslated(t *testing.T) {
	if DetectLocale("Item Class: Helmets\nRarity: Rare\nX\nY") != nil {
		t.Fatal("English text taken for another language")
	}
	if l := DetectLocale("Gegenstandsklasse: Helme\nSeltenheit: Selten\n"); l == nil || l.Lang != "de" {
		t.Fatal("German text not recognised")
	}
}

func TestGermanUniqueGemAndMagicNames(t *testing.T) {
	de := LocaleByLang("de")
	lines := []string{"Lichtbogen"}
	if class := de.title(lines, "gem"); lines[0] != "Arc" || class != "Skill Gems" {
		t.Fatalf("gem: %q %q", lines[0], class)
	}
	sceptre := localItemName(t, de, "Shrine Sceptre")
	lines = []string{sceptre + " der Zerstörung"}
	if class := de.title(lines, "magic"); lines[0] != "Shrine Sceptre" || class != "Sceptres" {
		t.Fatalf("magic: %q %q", lines[0], class)
	}
	var unique string
	for local, u := range de.uniques {
		if u.en == "Ab Aeterno" {
			unique = local
		}
	}
	if unique == "" {
		t.Skip("Ab Aeterno not in the tables")
	}
}

// TestGermanLinesWithTheRealCatalogs translates real German modifier lines
// with the trade site's two catalogs: POE2_DE_STATS names the German
// data/stats response; the English one is the app's cached copy.
func TestGermanLinesWithTheRealCatalogs(t *testing.T) {
	dePath := os.Getenv("POE2_DE_STATS")
	if dePath == "" {
		t.Skip("POE2_DE_STATS not set")
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
		return Catalog{Stats: r.Result, UpdatedAtMs: 2}
	}
	german := read(dePath)
	english := read(filepath.Join(os.Getenv("APPDATA"), "PoE2Filtre", "data", "trade_stats.json"))
	de := LocaleByLang("de")
	index, en := de.statIndex(german), englishStats(english)
	for local, want := range map[string]string{
		"+142 zu maximalem Leben":                 "+142 to maximum Life",
		"19% erhöhter Zauberschaden":              "19% increased Spell Damage",
		"+44(42-49) zu maximalem Leben":           "+44(42-49) to maximum Life",
		"+39(36-40)% zu Kältewiderstand":          "+39(36-40)% to Cold Resistance",
		"Fügt 10 bis 20 physischen Schaden hinzu": "Adds 10 to 20 Physical Damage",
	} {
		got, ok := englishStat(local, index, en)
		if !ok || got != want {
			t.Errorf("%q -> %q (%v), want %q", local, got, ok, want)
		}
	}
}

// TestRunePanelInJapanese: the recognizer returns Japanese one character a
// word ("lx 変 成 の オ ー ブ ( 上 級 )"); the rows still make the panel and
// name their rewards (lines as read off a player's screenshot).
func TestRunePanelInJapanese(t *testing.T) {
	ja := LocaleByLang("ja")
	names := ja.NamesFor([]string{"Greater Orb of Transmutation", "Greater Orb of Augmentation"})
	if len(names) != 2 {
		t.Fatalf("names %q", names)
	}
	line := func(text string, y, x, right float64) OcrLine {
		return OcrLine{Text: text, X: x, Y: y, W: right - x, H: 30}
	}
	lines := []OcrLine{
		line("lx 変 成 の オ ー ブ ( 上 級 )", 230, 680, 1013),
		line("lx 増 強 の オ ー ブ ( 上 級 )", 325, 680, 1012),
		line("サ ポ ー ト ジ ェ ム の 原 石", 420, 690, 1022),
		line("ス キ ル ジ ェ ム の 原 石", 515, 720, 1022),
	}
	rows, ok := FindRunePanel(lines, names)
	if !ok || len(rows) != 4 {
		t.Fatalf("panel %v, %d rows", ok, len(rows))
	}
	for i, want := range []string{"変成のオーブ (上級)", "増強のオーブ (上級)", "", ""} {
		if want != "" && ja.items[strings.ToLower(want)].en == "" {
			t.Fatalf("%q not in the tables", want)
		}
		if rows[i].Name != want || (want != "" && (!rows[i].CountRead || rows[i].Count != 1)) {
			t.Errorf("row %d: %q count %d (%v), want %q", i, rows[i].Name, rows[i].Count, rows[i].CountRead, want)
		}
	}
}

// TestRowCountsInOtherLanguages: Spanish writes the count after the name
// ("Runa de alcance x1", read "xl"), Russian in brackets ("(6)"), Chinese
// recognizers return "1 ×"; the characters' spaces come out of the text
// shown.
func TestRowCountsInOtherLanguages(t *testing.T) {
	for text, want := range map[string]struct {
		n    int
		rest string
	}{
		"Runa de alcance xl":   {1, "Runa de alcance"},
		"Orbe del artesano x3": {3, "Orbe del artesano"},
		"Точильный камень (6)": {6, "Точильный камень"},
		"Руна охвата (1":       {1, "Руна охвата"},
		"1 × 高 階 崇 高 石":        {1, "高 階 崇 高 石"},
		"3x 神 聖 石":             {3, "神 聖 石"},
	} {
		if n, rest := splitCount(text); n != want.n || rest != want.rest {
			t.Errorf("%q: %d %q, want %d %q", text, n, rest, want.n, want.rest)
		}
	}
	if got := compactCJK("lx 高 階 富 豪 石 (Greater Regal Orb)"); got != "lx 高階富豪石 (Greater Regal Orb)" {
		t.Errorf("compact: %q", got)
	}
}
