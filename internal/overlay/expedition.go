package overlay

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// runeshapeJSON is the Runeshape Combinations recipe table, built from
// PoE2DB by build/expedition/build_recipes.py (CC BY-NC-SA 3.0, see NOTICE).
//
//go:embed data/runeshape.json
var runeshapeJSON []byte

// RuneRecipe is one recipe: the reward, its count (1 when the panel shows
// none) and gem level, the area level band it rolls in, and its runes in
// order.
type RuneRecipe struct {
	Reward string   `json:"reward"`
	Count  int      `json:"count"`
	Level  int      `json:"level"`
	Tier   string   `json:"tier"`
	Runes  []string `json:"runes"`
}

var runeRecipes = sync.OnceValue(func() []RuneRecipe {
	var table struct {
		Recipes []RuneRecipe `json:"recipes"`
	}
	_ = json.Unmarshal(runeshapeJSON, &table)
	return table.Recipes
})

var tierRE = regexp.MustCompile(`^Lv(\d+)(?:-(\d+)|\+)$`)

// inTier tells whether a recipe can roll at the area level. Only the lower
// end of its band ("Lv30-74", "Lv70+") holds: a level 81 logbook offers
// Lv30-74 recipes too (3x Artificer's Orb, Greater Robust Rune). An unknown
// level or band lets every recipe through.
func inTier(tier string, areaLevel int) bool {
	m := tierRE.FindStringSubmatch(tier)
	if areaLevel <= 0 || m == nil {
		return true
	}
	lo, _ := strconv.Atoi(m[1])
	return areaLevel >= lo
}

// ResolveRuneCounts fills in the counts still unknown after the second read
// (see recountBox): when the recipes for that reward that can roll at the
// area's level all give the same count, that is the count. Most rewards have
// one recipe; Greater Orb of Transmutation is 1x from Lv30 and 3x from Lv70,
// so below level 70 it is 1x and above it stays unknown. A count that was
// read is kept, even one the table does not list (the table can lag a
// patch).
func ResolveRuneCounts(rows []RuneRow, areaLevel int) {
	recipes := runeRecipes()
	for i := range rows {
		row := &rows[i]
		if row.CountRead || row.Name == "" {
			continue
		}
		counts := map[int]bool{}
		for _, r := range recipes {
			if r.Reward == row.Name && inTier(r.Tier, areaLevel) {
				counts[r.Count] = true
			}
		}
		if len(counts) == 1 {
			for c := range counts {
				row.Count, row.CountRead = c, true
			}
		}
	}
}

// RuneRow is one reward of Expedition's Runeshape Combinations panel as read
// off the screen. Name is the currency item it grants, or "" for a reward that
// is none ("Rare Unique Item", "Unique Jewellery", a skill gem). Count is the
// reward's count; CountRead tells whether it is known, read or taken from
// the recipe table (ResolveRuneCounts), or only assumed to be 1 (the
// recognizer sometimes turns "3x" into noise). The box is the reward text's,
// in the pixels the lines came in; Right is where its words end.
type RuneRow struct {
	Text      string  `json:"text"`
	Name      string  `json:"name"`
	Count     int     `json:"count"`
	CountRead bool    `json:"countRead"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	W         float64 `json:"w"`
	H         float64 `json:"h"`
	Right     float64 `json:"right"`
}

// FindRunePanel picks the panel's rows out of the text read off the left of
// the screen. The panel is found by its rows, not its title: the title's
// hand-written font comes out as noise ("Ruwzshape COMİUÜOWS"), while every
// reward ends at the panel's right edge, a few pixels apart, whatever the
// noise at their left. Of the lines that end together, the set with the
// most rewards naming a currency item is the panel. A tooltip (the game
// opens one for the hovered row) and the "GAME PAUSED" banner are in
// capitals and left out; the rune icons and the life text are no words.
func FindRunePanel(lines []OcrLine, currencyNames []string) ([]RuneRow, bool) {
	names := nameKeys(currencyNames)
	if len(names) == 0 {
		return nil, false
	}
	var cands []RuneRow
	for _, line := range lines {
		if isCapsTitle(line.Text) {
			continue
		}
		count, rest := splitCount(line.Text)
		name, ok := matchRowName(rest, names)
		if !ok && !isWordy(rest) {
			continue
		}
		cands = append(cands, RuneRow{Text: line.Text, Name: name, Count: count, CountRead: rest != line.Text,
			X: line.X, Y: line.Y, W: line.W, H: line.H, Right: line.Right()})
	}
	var best []RuneRow
	bestNamed := 0
	for _, anchor := range cands {
		right := anchor.Right
		tolerance := max(4, anchor.H/2)
		var group []RuneRow
		named := 0
		for _, c := range cands {
			if math.Abs(c.Right-right) <= tolerance {
				group = append(group, c)
				if c.Name != "" {
					named++
				}
			}
		}
		if named > bestNamed || named == bestNamed && named > 0 && len(group) > len(best) {
			best, bestNamed = group, named
		}
	}
	if bestNamed == 0 {
		return nil, false
	}
	slices.SortFunc(best, func(a, b RuneRow) int { return cmp.Compare(a.Y, b.Y) })
	return best, true
}

// recountBoxes are the parts of the capture to read again for a row whose
// count was lost or whose name matched nothing: the reward's text alone,
// without the rune icons beside it, whose noise is what garbles "3x" ("M
// Greater Orb of Augmentation" reads "3x Greater Orb of Augmentation" this
// way). The text's width is estimated from the name (or the text read) and
// the letter width of the rows that read cleanly. The recognizer reads some
// crops and returns nothing for others of the same text, so a close crop
// comes first and one with more room around it next.
func recountBoxes(row RuneRow, rows []RuneRow) [][4]int {
	var heights, letters []float64
	for _, r := range rows {
		if !r.CountRead {
			continue
		}
		heights = append(heights, r.H)
		if n := len([]rune(r.Text)); n > 0 && r.Right > r.X {
			letters = append(letters, (r.Right-r.X)/float64(n))
		}
	}
	// A row whose box reached over its icons ("Ş 3x Greater Orb…") reads
	// too tall and too wide a letter, never too small: the low end is taken.
	h := lowEnd(heights, row.H)
	letter := lowEnd(letters, h/2)
	name := row.Name
	if name == "" {
		_, name = splitCount(row.Text)
	}
	width := letter * float64(len([]rune(name))+3) // "3x " and the name
	cy := row.Y + row.H/2
	box := func(left, around, right float64) [4]int {
		return [4]int{int(max(0, row.Right-width*1.08-h*left)), int(max(0, cy-around*h)), int(row.Right + h*right), int(cy + around*h)}
	}
	return [][4]int{box(0.3, 0.9, 0.4), box(1, 1.5, 1)}
}

// lowEnd is the lower quartile of the values (the smallest of three or
// fewer).
func lowEnd(values []float64, fallback float64) float64 {
	if len(values) == 0 {
		return fallback
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted[(len(sorted)-1)/4]
}

// mergeRunePanels joins the rows of two reads of the same panel: the
// recognizer now and then drops a row from one read that the other has. Rows
// at the same height are one row, and the better read of it is kept: a named
// reward over an unnamed one, a known count over an assumed one.
func mergeRunePanels(a, b []RuneRow) []RuneRow {
	out := slices.Clone(a)
	for _, row := range b {
		same := slices.IndexFunc(out, func(o RuneRow) bool {
			return math.Abs((o.Y+o.H/2)-(row.Y+row.H/2)) < max(o.H, row.H)*0.6
		})
		switch {
		case same < 0:
			out = append(out, row)
		case rowScore(row) > rowScore(out[same]):
			out[same] = row
		}
	}
	slices.SortFunc(out, func(x, y RuneRow) int { return cmp.Compare(x.Y, y.Y) })
	return out
}

func rowScore(r RuneRow) int {
	score := 0
	if r.Name != "" {
		score += 2
	}
	if r.CountRead {
		score++
	}
	return score
}

// recountFrom takes from the lines read off a recountBox a named row's
// count, or an unnamed row's name (and count): something passing over the
// panel as it was read, such as a line of dialogue, can garble a row.
func recountFrom(row *RuneRow, lines []OcrLine, names map[string]string) {
	for _, line := range lines {
		count, rest := splitCount(line.Text)
		counted := rest != line.Text
		name, ok := matchRowName(rest, names)
		if !ok || row.Name != "" && (name != row.Name || !counted) {
			continue
		}
		if row.Name == "" {
			row.Name, row.Text, row.Count = name, line.Text, 1
		}
		if counted {
			row.Count, row.CountRead = count, true
		}
		return
	}
}

// uncutGemRE is an uncut gem reward, "1x Uncut Spirit Gem (Level 19)", once
// folded to capitals; the type word is checked loosely ("SKILI"), the level
// only as digits.
var uncutGemRE = regexp.MustCompile(`UNCUT\s+(\S+)\s+GEM\s*\(\s*LEVEL\s*(\d{1,2})\s*\)`)

// UncutGem reads an uncut gem reward and returns its name as prices know it
// ("Uncut Skill Gem (Level 20)"). A gem's price differs several-fold from
// one level to the next, so a level that did not read as plain digits is no
// gem rather than a guess; one with no level printed ("Uncut Support Gem")
// is none either.
func UncutGem(text string) (string, bool) {
	m := uncutGemRE.FindStringSubmatch(strings.ToUpper(ocrFold.Replace(text)))
	if m == nil {
		return "", false
	}
	level, err := strconv.Atoi(m[2])
	if err != nil || level < 1 || level > 20 {
		return "", false
	}
	for _, kind := range []string{"Skill", "Spirit", "Support"} {
		if editDistance(strings.ToUpper(kind), m[1]) <= 1 {
			return fmt.Sprintf("Uncut %s Gem (Level %d)", kind, level), true
		}
	}
	return "", false
}
