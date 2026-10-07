package overlay

import (
	"slices"
	"strconv"
	"testing"
)

func rowSummary(rows []RuneRow) []string {
	var out []string
	for _, r := range rows {
		s := r.Name
		if s == "" {
			out = append(out, "? "+r.Text)
			continue
		}
		if r.Count != 1 || !r.CountRead {
			s += " ×" + string(rune('0'+r.Count))
			if !r.CountRead {
				s += "?"
			}
		}
		out = append(out, s)
	}
	return out
}

func TestFindRunePanelSagas(t *testing.T) {
	rows, ok := FindRunePanel(runeshapeLines, someCurrencyNames)
	want := []string{"Olroth's Saga", "Vorana's Saga", "Uhtred's Saga", "Medved's Saga", "Artificer's Orb ×3",
		"Greater Robust Rune", "Greater Resolve Rune", "Greater Adept Rune"}
	if got := rowSummary(rows); !ok || !slices.Equal(got, want) {
		t.Fatalf("got %q, %v; want %q", got, ok, want)
	}
}

func TestFindRunePanelSkipsTheTooltipAndKeepsOtherRewards(t *testing.T) {
	names := append(slices.Clone(someCurrencyNames), "Prismatic Alloy", "Greater Orb of Transmutation", "Greater Orb of Augmentation")
	rows, ok := FindRunePanel(alloyLines, names)
	want := []string{"Prismatic Alloy", "? Rare Unique item", "Greater Orb of Transmutation ×3", "Greater Orb of Augmentation ×1?"}
	if got := rowSummary(rows); !ok || !slices.Equal(got, want) {
		t.Fatalf("got %q, %v; want %q", got, ok, want)
	}
}

func TestFindRunePanelNeedsARewardRow(t *testing.T) {
	if rows, ok := FindRunePanel(skillsPanelLines, someCurrencyNames); ok {
		t.Fatalf("found a panel in the Skills panel: %+v", rows)
	}
}

// A scrollbar's edge read as a mark after the name ("Masterwork Rune )")
// must not move the row's right edge off the panel's.
func TestFindRunePanelIgnoresAMarkAfterTheName(t *testing.T) {
	lines := []OcrLine{
		{Text: "IX Mystic Alloy", X: 414, Y: 124, W: 132, H: 22},
		{Text: "IX Masterwork Rune )", X: 363, Y: 174, W: 217, H: 19, TextRight: 546},
		{Text: "Ward", X: 19, Y: 816, W: 47, H: 15},
	}
	rows, ok := FindRunePanel(lines, []string{"Mystic Alloy", "Masterwork Rune"})
	if got := rowSummary(rows); !ok || !slices.Equal(got, []string{"Mystic Alloy", "Masterwork Rune"}) {
		t.Fatalf("got %q, %v", got, ok)
	}
}

func TestResolveRuneCountsByAreaLevel(t *testing.T) {
	summary := func(rows []RuneRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.Name+" "+strconv.Itoa(r.Count)+" "+strconv.FormatBool(r.CountRead))
		}
		return out
	}
	rows := []RuneRow{
		{Name: "Greater Orb of Transmutation", Count: 1},                 // 1x from Lv30, 3x from Lv70
		{Name: "Greater Orb of Augmentation", Count: 3, CountRead: true}, // read: kept
		{Name: "Masterwork Rune", Count: 1},                              // one recipe
		{Name: "", Text: "Rare Unique item", Count: 1},                   // no currency
	}
	ResolveRuneCounts(rows, 81)
	want := []string{"Greater Orb of Transmutation 1 false", "Greater Orb of Augmentation 3 true", "Masterwork Rune 1 true", " 1 false"}
	if got := summary(rows); !slices.Equal(got, want) {
		t.Fatalf("level 81: got %q, want %q", got, want)
	}
	// Below level 70 only the 1x recipe can roll.
	rows = []RuneRow{{Name: "Greater Orb of Transmutation", Count: 1}}
	ResolveRuneCounts(rows, 50)
	if got := summary(rows); !slices.Equal(got, []string{"Greater Orb of Transmutation 1 true"}) {
		t.Fatalf("level 50: got %q", got)
	}
}

func TestRuneRecipeTableIsEmbedded(t *testing.T) {
	if n := len(runeRecipes()); n < 300 {
		t.Fatalf("%d recipes", n)
	}
	if !inTier("Lv70+", 81) || inTier("Lv70+", 65) || !inTier("Lv30-74", 81) || !inTier("Lv30-74", 0) {
		t.Fatal("inTier")
	}
}

func TestRecountBoxCoversTheTextOnly(t *testing.T) {
	names := append(slices.Clone(someCurrencyNames), "Prismatic Alloy", "Greater Orb of Transmutation", "Greater Orb of Augmentation")
	rows, _ := FindRunePanel(alloyLines, names)
	aug := rows[3]
	b := recountBoxes(aug, rows)[0]
	x0, y0, x1, y1 := b[0], b[1], b[2], b[3]
	// The text "3x Greater Orb of Augmentation" ends at 712; at about 12 px
	// a letter it starts near 340, while the box reaches the icons at 41.
	if x0 < 250 || x0 > 360 || x1 < 712 || x1 > 730 || y0 > 410 || y1 < 445 {
		t.Fatalf("box %d,%d - %d,%d", x0, y0, x1, y1)
	}
	recountFrom(&aug, []OcrLine{{Text: "3x Greater Orb of Augmentation"}}, nameKeys(names))
	if aug.Count != 3 || !aug.CountRead {
		t.Fatalf("recount: %+v", aug)
	}
}

func TestMergeRunePanelsKeepsTheBetterRead(t *testing.T) {
	a := []RuneRow{
		{Name: "Prismatic Alloy", Count: 1, CountRead: true, Y: 185, H: 25},
		{Name: "Greater Orb of Augmentation", Count: 1, Y: 350, H: 44},
	}
	b := []RuneRow{
		{Name: "", Text: "Rare Unique item", Count: 1, Y: 246, H: 22},
		{Name: "Greater Orb of Augmentation", Count: 3, CountRead: true, Y: 355, H: 21},
		{Name: "Prismatic Alloy", Count: 1, Y: 186, H: 25},
	}
	got := rowSummary(mergeRunePanels(a, b))
	want := []string{"Prismatic Alloy", "? Rare Unique item", "Greater Orb of Augmentation ×3"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A row garbled as it was read (a line of dialogue over "1x Ward Rune")
// takes its name from the second read of its text.
func TestRecountNamesAGarbledRow(t *testing.T) {
	row := RuneRow{Text: "Right then—who n get Rune", Count: 1}
	recountFrom(&row, []OcrLine{{Text: "IX Ward Rune"}}, nameKeys([]string{"Ward Rune", "Mind Rune"}))
	if row.Name != "Ward Rune" || row.Count != 1 || !row.CountRead {
		t.Fatalf("got %+v", row)
	}
	// Text that names nothing leaves the row unnamed.
	row = RuneRow{Text: "Rare Unique item", Count: 1}
	recountFrom(&row, []OcrLine{{Text: "Rare Unique item"}}, nameKeys([]string{"Ward Rune"}))
	if row.Name != "" {
		t.Fatalf("got %+v", row)
	}
}

func TestUncutGem(t *testing.T) {
	for text, want := range map[string]string{
		"IX Uncut Skili Gem (Level 20)":   "Uncut Skill Gem (Level 20)",
		"IX Uncut spirit Gem (Level 19)":  "Uncut Spirit Gem (Level 19)",
		"1x Uncut Support Gem (Level 4)":  "Uncut Support Gem (Level 4)",
		"IX Uncut Spirit Gem (Level 2O)":  "", // the level must read as digits
		"Uncut Support Gem":               "", // no level printed
		"IX Thaumaturgic Flux (Level 20)": "",
	} {
		got, _ := UncutGem(text)
		if got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}
