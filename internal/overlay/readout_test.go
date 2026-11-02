package overlay

import (
	"image"
	"testing"
)

func TestParseExperience(t *testing.T) {
	cases := []struct {
		name  string
		lines []OcrLine
		want  Experience
		ok    bool
	}{
		{"one row", []OcrLine{
			{Text: "Level 99 (1%)", Y: 1078, H: 20},
			{Text: "Current Exp: 3.937.469.451 / Next Level: 4.250.334.444", Y: 1101, H: 20},
		}, Experience{3937469451, 4250334444}, true},
		{"cut in two, the slash read as a one", []OcrLine{
			{Text: "Level 99 (1%)", X: 919, Y: 21, H: 20},
			{Text: "Current Exp: 3.937.469.451 1 Next Level:", X: 918, Y: 44, H: 20},
			{Text: "4.250.334.444", X: 1265, Y: 47, H: 14},
		}, Experience{3937469451, 4250334444}, true},
		{"the rate row below, once experience was gained", []OcrLine{
			{Text: "Level 99 (1%)", Y: 0, H: 20},
			{Text: "Current Exp: 3.937.591.990 / Next Level: 4.250.334.444", Y: 24, H: 20},
			{Text: "Exp Per Hour: 2.267.700", Y: 48, H: 20},
		}, Experience{3937591990, 4250334444}, true},
		{"commas", []OcrLine{
			{Text: "Level 9 (55%)", Y: 0, H: 20},
			{Text: "Current Exp: 12,345 / Next Level: 23,000", Y: 24, H: 20},
		}, Experience{12345, 23000}, true},
		{"spaces", []OcrLine{{Text: "Опыт: 3 937 469 451 / 4 250 334 444", Y: 0, H: 20}}, Experience{3937469451, 4250334444}, true},
		{"small numbers", []OcrLine{{Text: "Current Exp: 525 / Next Level: 1,760", Y: 0, H: 20}}, Experience{525, 1760}, true},
		{"only the level row", []OcrLine{{Text: "Level 9 (55%)", Y: 0, H: 20}, {Text: "Current Exp:", Y: 24, H: 20}}, Experience{}, false},
		{"nothing", []OcrLine{{Text: "Highlight Items", Y: 0, H: 20}}, Experience{}, false},
		{"current above next is a misread", []OcrLine{{Text: "Current Exp: 9.000 / Next Level: 8.000", Y: 0, H: 20}}, Experience{}, false},
	}
	for _, c := range cases {
		got, ok := ParseExperience(c.lines)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestParseGold(t *testing.T) {
	for text, want := range map[string]int64{"72.978.464": 72978464, "72978.464": 72978464, "1,250": 1250, "0": 0} {
		if got, ok := ParseGold([]OcrLine{{Text: text, H: 20}}); !ok || got != want {
			t.Errorf("%q: got %d %v, want %d", text, got, ok, want)
		}
	}
	if _, ok := ParseGold([]OcrLine{{Text: "Gold", H: 20}}); ok {
		t.Error("a row without a number is no gold")
	}
}

func TestEnlargeKeepsFlatColours(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 200, 100, 50, 255
	}
	out := enlarge(img, image.Rect(1, 1, 3, 3), 3)
	if out.Rect.Dx() != 6 || out.Rect.Dy() != 6 {
		t.Fatalf("size %v", out.Rect)
	}
	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 200 || out.Pix[i+1] != 100 || out.Pix[i+2] != 50 || out.Pix[i+3] != 255 {
			t.Fatalf("pixel %d: %v", i/4, out.Pix[i:i+4])
		}
	}
}
