package overlay

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// OcrLine is one line of text read off the screen, boxed in screen pixels.
type OcrLine struct {
	Text       string
	X, Y, W, H float64
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

// GemFromText finds the gem whose tooltip the cursor shows and writes it as
// the text the game would have copied, so the usual parser takes it from
// there. The Skills panel prints other gem names too (its rows); the
// tooltip's title is the one with the tooltip's next line straight under it,
// starting at the same edge, and of those the nearest to the cursor wins.
func GemFromText(lines []OcrLine, cursorX, cursorY float64, gemNames []string) (string, bool) {
	names := make(map[string]string, len(gemNames))
	for _, name := range gemNames {
		if key := ocrKey(name); len(key) >= 4 {
			names[key] = name
		}
	}
	best, bestScore, bestDistance := -1, -1, math.Inf(1)
	var bestName string
	for i, line := range lines {
		name, ok := matchGemName(ocrKey(line.Text), names)
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
	if best < 0 {
		return "", false
	}
	var b strings.Builder
	b.WriteString("Rarity: Gem\n")
	b.WriteString(bestName + "\n--------\n")
	if below := lineBelow(lines, best); below >= 0 {
		// The tag line tells a support gem from a skill gem (gemClass).
		if strings.Contains(ocrKey(lines[below].Text), "SUPPORT") {
			b.WriteString("Support\n")
		} else {
			b.WriteString(lines[below].Text + "\n")
		}
	}
	if level := tooltipLevel(lines, best); level != "" {
		b.WriteString("Level: " + level + "\n")
	}
	return b.String(), true
}

func matchGemName(key string, names map[string]string) (string, bool) {
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
