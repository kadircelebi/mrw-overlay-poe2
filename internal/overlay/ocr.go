package overlay

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// OcrLine is one line of text read off the screen, boxed in screen pixels.
// TextRight is the right edge of its last real word (see isRealWord); 0 when
// unknown.
type OcrLine struct {
	Text       string
	X, Y, W, H float64
	TextRight  float64
}

// Right is where the line's text ends, a stray mark after it left out.
func (l OcrLine) Right() float64 {
	if l.TextRight > 0 {
		return l.TextRight
	}
	return l.X + l.W
}

// isRealWord tells a word from a stray mark: two letters or digits.
func isRealWord(text string) bool {
	n := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n >= 2
}

var ocrLevelRE = regexp.MustCompile(`^LEVEL\s*:?\s*(\d+)`)

// ocrFold maps what the recognizer tends to see in the game's small capitals
// to plain letters: a Turkish recognizer dots and hooks them ("COŞT",
// "MULTİPLIER") and the serif "I" comes out as "/" ("SK/LL").
var ocrFold = strings.NewReplacer(
	"İ", "I", "ı", "I", "Ş", "S", "ş", "S", "Ğ", "G", "ğ", "G",
	"Ü", "U", "ü", "U", "Ö", "O", "ö", "O", "Ç", "C", "ç", "C",
	"/", "I", "|", "I",
)

func ocrKey(s string) string {
	s = strings.ToUpper(ocrFold.Replace(s))
	var b strings.Builder
	for _, r := range s {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ScreenItemText is the item under the cursor read off the screen, for the
// panels the game copies nothing from: a gem in the Skills panel, or a reward
// of Expedition's Runeshape Combinations (runes, alloys, sagas, orbs; all in
// the currency catalog). It is written as the text the game would have
// copied, so the usual parser takes it from there. With both kinds on screen
// the line nearer the cursor wins, so a "Skill Level 20: …" reward row does
// not beat the rune row under the cursor.
func ScreenItemText(lines []OcrLine, cursorX, cursorY float64, gemNames, currencyNames []string) (string, bool) {
	gem, gemName := gemTitle(lines, cursorX, cursorY, nameKeys(gemNames))
	cur, curName, count := currencyLine(lines, cursorX, cursorY, nameKeys(currencyNames))
	switch {
	case cur >= 0 && (gem < 0 || cursorDistance(lines[cur], cursorX, cursorY) < cursorDistance(lines[gem], cursorX, cursorY)):
		return currencyText(curName, count), true
	case gem >= 0:
		return gemText(lines, gem, gemName), true
	}
	return "", false
}

func nameKeys(list []string) map[string]string {
	names := make(map[string]string, len(list))
	for _, name := range list {
		if key := ocrKey(name); len(key) >= 4 {
			names[key] = name
		}
	}
	return names
}

// gemTitle is the line holding the title of the gem tooltip the cursor
// shows, or -1. The Skills panel prints other gem names too (its rows); the
// tooltip's title is the one with the tooltip's next line straight under it,
// starting at the same edge, and of those the nearest to the cursor wins.
func gemTitle(lines []OcrLine, cursorX, cursorY float64, names map[string]string) (int, string) {
	best, bestScore, bestDistance := -1, -1, math.Inf(1)
	var bestName string
	if len(names) == 0 {
		return best, bestName
	}
	for i, line := range lines {
		name, ok := matchOcrName(ocrKey(line.Text), names)
		if !ok {
			continue
		}
		score := 0
		if below := lineBelow(lines, i); below >= 0 {
			score = 2
			if strings.Contains(ocrKey(lines[below].Text), "SUPPORT") || strings.Contains(lines[below].Text, ",") {
				score = 3
			}
		}
		distance := math.Hypot(line.X+line.W/2-cursorX, line.Y+line.H/2-cursorY)
		if score > bestScore || score == bestScore && distance < bestDistance {
			best, bestScore, bestDistance, bestName = i, score, distance, name
		}
	}
	return best, bestName
}

func gemText(lines []OcrLine, title int, name string) string {
	var b strings.Builder
	b.WriteString("Rarity: Gem\n")
	b.WriteString(name + "\n--------\n")
	if below := lineBelow(lines, title); below >= 0 {
		// The tag line tells a support gem from a skill gem (gemClass).
		if strings.Contains(ocrKey(lines[below].Text), "SUPPORT") {
			b.WriteString("Support\n")
		} else {
			b.WriteString(lines[below].Text + "\n")
		}
	}
	if level := tooltipLevel(lines, title); level != "" {
		b.WriteString("Level: " + level + "\n")
	}
	return b.String()
}

// ocrCountRE is a reward row's count, "3x Artificer's Orb". The recognizer
// reads the 1 and 0 of the game's font as letters ("1x" comes out "IX",
// "10x" "IOX") and the x now and then as ")'" ("3)'").
var ocrCountRE = regexp.MustCompile(`^\s*(?:\S{1,2}\s+)?([0-9IiLl|Oo]{1,3})\s*[xX)'’]+\s+`)

var ocrDigits = strings.NewReplacer("I", "1", "i", "1", "L", "1", "l", "1", "|", "1", "O", "0", "o", "0")

// splitCount takes the count off the front of a reward row: 1 when there is
// none (a tooltip title, or a count misread past recognition).
func splitCount(text string) (int, string) {
	m := ocrCountRE.FindStringSubmatch(text)
	if m == nil {
		return 1, text
	}
	n, err := strconv.Atoi(ocrDigits.Replace(m[1]))
	if err != nil || n < 1 {
		n = 1
	}
	return n, text[len(m[0]):]
}

// matchRowName names the item a row ends with. The row's icons sometimes come
// out as a short token in front of the name ("M Greater Orb of
// Augmentation"); up to two such tokens are dropped when the whole does not
// match. A stray mark after the name ("Masterwork Rune )") goes first.
func matchRowName(text string, names map[string]string) (string, bool) {
	tokens := strings.Fields(text)
	for len(tokens) > 1 && !isRealWord(tokens[len(tokens)-1]) {
		tokens = tokens[:len(tokens)-1]
	}
	for drop := 0; drop <= 2 && drop < len(tokens); drop++ {
		if drop > 0 && len([]rune(tokens[drop-1])) > 3 {
			break
		}
		if name, ok := matchOcrName(ocrKey(strings.Join(tokens[drop:], " ")), names); ok {
			return name, true
		}
	}
	return "", false
}

// isCapsTitle tells a tooltip title: the game prints it in small capitals,
// which the recognizer returns as capitals ("PRISMATIC ALLOY"), while the
// panel's rows are in mixed case.
func isCapsTitle(text string) bool {
	letters, upper := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 4 && upper*10 >= letters*8
}

// currencyLine is the line naming the currency item the cursor points at, or
// -1, with the reward's count. The tooltip the game opens for the hovered
// row names the item in capitals, and there is only one, so it wins; its
// count comes from the row of that name ("1x Medved's Saga"). Without it the
// nearest row is taken: rows lie one under another, so height counts more
// than width. That alone can miss on a two-line row (icons above, name
// below), where the cursor on the icons is about as near the row above.
// A row counts within three of its heights above or below the cursor and
// three of its widths aside (the cursor may be on the row's icons); the
// tooltip opens well to the side, so only its height is bounded.
func currencyLine(lines []OcrLine, cursorX, cursorY float64, names map[string]string) (int, string, int) {
	type hit struct {
		line     int
		name     string
		count    int
		distance float64
	}
	var rows []hit
	title := hit{line: -1, distance: math.Inf(1)}
	other := math.Inf(1) // the nearest row naming something else
	if len(names) == 0 {
		return -1, "", 1
	}
	for i, line := range lines {
		dx, dy := math.Abs(line.X+line.W/2-cursorX), math.Abs(line.Y+line.H/2-cursorY)
		count, rest := splitCount(line.Text)
		isTitle := rest == line.Text && isCapsTitle(line.Text)
		if isTitle && dy > 6*line.H || !isTitle && (dy > 3*line.H || dx > 3*line.W) {
			continue
		}
		name, ok := matchRowName(rest, names)
		if !ok {
			if !isTitle && isWordy(rest) {
				other = min(other, cursorDistance(line, cursorX, cursorY))
			}
			continue
		}
		h := hit{i, name, count, cursorDistance(line, cursorX, cursorY)}
		if isTitle {
			if h.distance < title.distance {
				title = h
			}
		} else {
			rows = append(rows, h)
		}
	}
	slices.SortFunc(rows, func(a, b hit) int { return cmp.Compare(a.distance, b.distance) })
	if title.line >= 0 {
		for _, row := range rows {
			if row.name == title.name {
				return title.line, title.name, row.count
			}
		}
		return title.line, title.name, 1
	}
	// The cursor on a reward that is no currency ("Rare Unique Item") must
	// not price the row beside it.
	if len(rows) > 0 && rows[0].distance < other {
		return rows[0].line, rows[0].name, rows[0].count
	}
	return -1, "", 1
}

// isWordy tells a line of words (a reward row) from the noise the row's
// icons come out as ("ZŞ89", "mm"): two words of three letters or more.
func isWordy(text string) bool {
	words := 0
	for _, field := range strings.Fields(text) {
		letters := 0
		for _, r := range field {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters >= 3 {
			words++
		}
	}
	return words >= 2
}

// cursorDistance weighs height far over width: the panel's rows lie one
// under another, and a row's box may reach over its icons to the left.
func cursorDistance(line OcrLine, cursorX, cursorY float64) float64 {
	return math.Hypot((line.X+line.W/2-cursorX)/10, line.Y+line.H/2-cursorY)
}

// currencyText writes a currency item as the game would have copied it; a
// reward's count becomes its stack, so the card shows the reward's worth.
func currencyText(name string, count int) string {
	text := "Item Class: Stackable Currency\nRarity: Currency\n" + name + "\n--------\n"
	if count > 1 {
		text += fmt.Sprintf("Stack Size: %d/%d\n", count, count)
	}
	return text
}

func matchOcrName(key string, names map[string]string) (string, bool) {
	if len(key) < 4 {
		return "", false
	}
	if name, ok := names[key]; ok {
		return name, true
	}
	// One misread letter in eight is forgiven ("RAKIATA'S FL0W").
	allowed := len(key) / 8
	if allowed == 0 {
		return "", false
	}
	found, foundDistance := "", allowed+1
	for k, name := range names {
		if abs(len(k)-len(key)) > allowed {
			continue
		}
		if d := editDistance(k, key); d < foundDistance {
			found, foundDistance = name, d
		}
	}
	return found, found != ""
}

// lineBelow is the line right under lines[i] that starts at its left edge:
// in a tooltip every line lines up with the title.
func lineBelow(lines []OcrLine, i int) int {
	title := lines[i]
	best, bestGap := -1, math.Inf(1)
	for j, line := range lines {
		if j == i {
			continue
		}
		gap := line.Y - (title.Y + title.H)
		if gap < 0 || gap > 2.5*title.H || math.Abs(line.X-title.X) > 0.8*title.H {
			continue
		}
		if gap < bestGap {
			best, bestGap = j, gap
		}
	}
	return best
}

// tooltipLevel is a "Level: N" line in the tooltip under the title.
func tooltipLevel(lines []OcrLine, title int) string {
	t := lines[title]
	for _, line := range lines {
		if line.Y <= t.Y || line.Y > t.Y+20*t.H || math.Abs(line.X-t.X) > 0.8*t.H {
			continue
		}
		upper := strings.ToUpper(ocrFold.Replace(line.Text))
		if m := ocrLevelRE.FindStringSubmatch(upper); m != nil {
			return m[1]
		}
	}
	return ""
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// unrotate maps line boxes back onto the image the recognizer was given.
// When it finds the text slanted (TextAngle, degrees clockwise) it reads a
// straightened copy and boxes the words there; on the game's level text it
// now and then finds a slant of a few degrees that is not there (4.6° on a
// 1344 px wide 1080p capture), which shifts the boxes' right edges by tens of
// pixels from row to row. Turning the boxes by the angle around the image's
// centre (cx, cy) puts them back.
func unrotate(lines []OcrLine, angle, cx, cy float64) {
	rad := angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	turn := func(x, y float64) (float64, float64) {
		dx, dy := x-cx, y-cy
		return cx + dx*cos - dy*sin, cy + dx*sin + dy*cos
	}
	for i := range lines {
		l := &lines[i]
		x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, p := range [][2]float64{{l.X, l.Y}, {l.X + l.W, l.Y}, {l.X, l.Y + l.H}, {l.X + l.W, l.Y + l.H}} {
			x, y := turn(p[0], p[1])
			x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
		}
		textRight, _ := turn(l.TextRight, l.Y+l.H/2)
		l.X, l.Y, l.W, l.H, l.TextRight = x0, y0, x1-x0, y1-y0, textRight
	}
}
