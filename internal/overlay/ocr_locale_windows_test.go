//go:build windows

package overlay

import (
	"image"
	"image/draw"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type gemShot struct {
	base, class string
	level       int
}

// TestGemTooltipsFromScreenshots reads Skills panel gem tooltips of game
// clients in other languages: POE2_DE_DIR and POE2_FR_DIR name folders of
// screenshots (none are kept in the repository) whose names end in the keys
// below.
func TestGemTooltipsFromScreenshots(t *testing.T) {
	t.Run("de", func(t *testing.T) {
		checkGemShots(t, "de", os.Getenv("POE2_DE_DIR"), map[string]gemShot{
			"183959": {"Cool Headed", "Support Gems", 0},
			"184006": {"Her Declaration", "Support Gems", 0},
			"184010": {"Seraph's Heart", "Support Gems", 0},
			"184018": {"Arc", "Skill Gems", 40},
			"184030": {"Urgent Totems III", "Support Gems", 0},
			"184035": {"Spell Totem", "Skill Gems", 23},
		})
	})
	t.Run("fr", func(t *testing.T) {
		checkGemShots(t, "fr", os.Getenv("POE2_FR_DIR"), map[string]gemShot{
			"(1)": {"Clarity II", "Support Gems", 0},
			"(2)": {"Her Declaration", "Support Gems", 0},
			"(3)": {"Seraph's Heart", "Support Gems", 0},
			"(4)": {"Urgent Totems III", "Support Gems", 0},
		})
	})
}

func checkGemShots(t *testing.T, lang, dir string, want map[string]gemShot) {
	if dir == "" {
		t.Skip("no screenshot folder")
	}
	loc := LocaleByLang(lang)
	SetOCRLanguage(OCRTags[lang])
	defer SetOCRLanguage("")
	read := func(path string) (Item, bool) {
		f, _ := os.Open(path)
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		pixels := make([]byte, len(rgba.Pix))
		for i := 0; i < len(pixels); i += 4 {
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = rgba.Pix[i+2], rgba.Pix[i+1], rgba.Pix[i], 255
		}
		lines, err := recognize(pixels, b.Dx(), b.Dy())
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("POE2_OCR_LINES") != "" {
			for _, l := range lines[:min(4, len(lines))] {
				t.Logf("%s: line %q", filepath.Base(path), l.Text)
			}
		}
		text, ok := ScreenItemTextIn(lines, 150, 40, loc.GemNames(), nil, loc.ScreenWords())
		if !ok {
			var seen []string
			for _, l := range lines {
				seen = append(seen, l.Text)
			}
			t.Logf("%s: read %q", filepath.Base(path), seen)
			return Item{}, false
		}
		parts := strings.Split(text, "\n")
		parts[1], _ = loc.EnglishName(parts[1])
		item, err := ParseItem(strings.Join(parts, "\n"), Catalog{})
		if err != nil {
			t.Fatal(err)
		}
		return item, true
	}
	if len(want) == 0 {
		// No expectations yet: show what is read.
		paths, _ := filepath.Glob(filepath.Join(dir, "*.png"))
		for _, p := range paths {
			item, ok := read(p)
			t.Logf("%s: %v %q class %q level %d", filepath.Base(p), ok, item.BaseType, item.Class, item.GemLevel)
		}
		return
	}
	for key, w := range want {
		paths, _ := filepath.Glob(filepath.Join(dir, "*"+key+".png"))
		if len(paths) == 0 {
			t.Fatalf("%s: no screenshot", key)
		}
		item, ok := read(paths[0])
		if !ok || item.BaseType != w.base || item.Class != w.class || item.GemLevel != w.level {
			t.Errorf("%s: %q class %q level %d, want %+v", key, item.BaseType, item.Class, item.GemLevel, w)
		}
	}
}
