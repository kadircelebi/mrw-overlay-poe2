package overlay

import (
	"image"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Experience is what the game's experience bar says with the pointer on it:
// the character's total experience and the total the next level takes.
type Experience struct {
	Current, Next int64
}

// readoutNumber is a number as the game writes it, whatever the Windows
// region's separator: "3.937.469.451", "3,937,469,451", "3 937 469 451". The
// recognizer may lose a separator ("72978.464") and read the slash between
// the two experience figures as a lone 1, so groups are not held to three
// figures except across a space.
var readoutNumber = regexp.MustCompile(`\d+(?:[.,'\x{00A0}\x{202F}]\d+|\s\d{3}\b)*`)

func readoutNumbers(text string) []int64 {
	var out []int64
	for _, m := range readoutNumber.FindAllString(text, -1) {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, m)
		if n, err := strconv.ParseInt(digits, 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// readoutRows joins the lines standing at the same height (the recognizer
// may cut one row in two), top to bottom.
func readoutRows(lines []OcrLine) []string {
	sorted := append([]OcrLine(nil), lines...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Y < sorted[j].Y })
	var rows [][]OcrLine
	for _, l := range sorted {
		mid := l.Y + l.H/2
		if n := len(rows); n > 0 {
			last := rows[n-1][0]
			if mid >= last.Y && mid <= last.Y+last.H {
				rows[n-1] = append(rows[n-1], l)
				continue
			}
		}
		rows = append(rows, []OcrLine{l})
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		sort.Slice(row, func(a, b int) bool { return row[a].X < row[b].X })
		parts := make([]string, len(row))
		for j, l := range row {
			parts[j] = l.Text
		}
		out[i] = strings.Join(parts, " ")
	}
	return out
}

// ParseExperience finds the experience figures in the lines read off the
// bar's tooltip: its lowest row with two numbers, the first and the last of
// them ("Current Exp: A / Next Level: B", in any of the game's languages).
// The row above it ("Level 99 (1%)", the one with a percent sign) is never
// taken.
func ParseExperience(lines []OcrLine) (Experience, bool) {
	rows := readoutRows(lines)
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.Contains(rows[i], "%") {
			continue
		}
		nums := readoutNumbers(rows[i])
		if len(nums) < 2 {
			continue
		}
		cur, next := nums[0], nums[len(nums)-1]
		if cur < next {
			return Experience{Current: cur, Next: next}, true
		}
		return Experience{}, false
	}
	return Experience{}, false
}

// ParseGold finds the gold figure in the lines read off the box under the
// inventory: a row with a single number.
func ParseGold(lines []OcrLine) (int64, bool) {
	for _, row := range readoutRows(lines) {
		if nums := readoutNumbers(row); len(nums) == 1 {
			return nums[0], true
		}
	}
	return 0, false
}

// readoutScales are the sizes a part of a picture of the given height is
// read at: small text reads better enlarged.
func readoutScales(height int) []int {
	f := 1
	if height > 0 {
		f = (2160 + height - 1) / height
	}
	return []int{f, f + 1}
}

// enlarge is the part r of img at f times its size, smoothly.
func enlarge(img *image.RGBA, r image.Rectangle, f int) *image.RGBA {
	r = r.Intersect(img.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, r.Dx()*f, r.Dy()*f))
	if r.Empty() {
		return out
	}
	at := func(x, y, c int) int {
		x, y = min(max(x, r.Min.X), r.Max.X-1), min(max(y, r.Min.Y), r.Max.Y-1)
		return int(img.Pix[img.PixOffset(x, y)+c])
	}
	for y := 0; y < out.Rect.Dy(); y++ {
		// In f-ths of a source pixel, from its centre.
		sy := y*2 + 1 - f
		y0, fy := r.Min.Y+floorDiv(sy, 2*f), floorMod(sy, 2*f)
		for x := 0; x < out.Rect.Dx(); x++ {
			sx := x*2 + 1 - f
			x0, fx := r.Min.X+floorDiv(sx, 2*f), floorMod(sx, 2*f)
			o := out.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				top := at(x0, y0, c)*(2*f-fx) + at(x0+1, y0, c)*fx
				bottom := at(x0, y0+1, c)*(2*f-fx) + at(x0+1, y0+1, c)*fx
				out.Pix[o+c] = uint8((top*(2*f-fy) + bottom*fy) / (4 * f * f))
			}
			out.Pix[o+3] = 255
		}
	}
	return out
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

func floorMod(a, b int) int { return a - floorDiv(a, b)*b }

// readoutOf reads the part r of img at each scale with parse; every read
// that finds something must find the same, and one must.
func readoutOf[T comparable](img *image.RGBA, r image.Rectangle, height int, parse func([]OcrLine) (T, bool)) (T, bool) {
	var found, zero T
	seen := false
	for _, f := range readoutScales(height) {
		lines, err := recognizeRGBA(enlarge(img, r, f))
		if err != nil {
			return zero, false
		}
		v, ok := parse(lines)
		if !ok {
			continue
		}
		if seen && v != found {
			return zero, false
		}
		found, seen = v, true
	}
	return found, seen
}

// ReadExperience reads the experience figures off the part r of img, a
// picture of a game client height pixels high.
func ReadExperience(img *image.RGBA, r image.Rectangle, height int) (Experience, bool) {
	return readoutOf(img, r, height, ParseExperience)
}

// ReadGold reads the gold figure off the part r of img, a picture of a game
// client height pixels high.
func ReadGold(img *image.RGBA, r image.Rectangle, height int) (int64, bool) {
	return readoutOf(img, r, height, ParseGold)
}
