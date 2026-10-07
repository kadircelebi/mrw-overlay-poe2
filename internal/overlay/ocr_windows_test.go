//go:build windows

package overlay

import (
	"encoding/json"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRecognizeScreenshot runs Windows' recognizer on a saved screenshot:
// POE2_OCR_SAMPLE=<png or jpg> names it (none is kept in the repository).
func TestRecognizeScreenshot(t *testing.T) {
	path := os.Getenv("POE2_OCR_SAMPLE")
	if path == "" {
		t.Skip("POE2_OCR_SAMPLE not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	pixels := make([]byte, len(rgba.Pix))
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = rgba.Pix[i+2], rgba.Pix[i+1], rgba.Pix[i], 255
	}
	start := time.Now()
	lines, err := recognize(pixels, rgba.Bounds().Dx(), rgba.Bounds().Dy())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d lines in %v", len(lines), time.Since(start))
	for _, line := range lines {
		t.Logf("%5.0f %5.0f %4.0f %3.0f  %s", line.X, line.Y, line.W, line.H, line.Text)
	}
	if raw, ok := ScreenItemText(lines, 380, 480, []string{"Archmage", "Virtuous Barrier", "Rakiata's Flow"}, nil); ok {
		t.Logf("item:\n%s", raw)
	}
}

// TestReadRunePanelScreenshots reads Expedition's Runeshape panel off saved
// full-window screenshots as the price labels would:
// POE2_RUNE_SAMPLES=<png or jpg>;<…> names them, and the currency names come
// from the trade catalog in %APPDATA%\PoE2Filtre\data (none are kept in the
// repository).
func TestReadRunePanelScreenshots(t *testing.T) {
	samples := os.Getenv("POE2_RUNE_SAMPLES")
	if samples == "" {
		t.Skip("POE2_RUNE_SAMPLES not set")
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "PoE2Filtre", "data", "trade_items.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items response[ItemGroup]
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range items.Result {
		if g.ID == "currency" {
			for _, e := range g.Entries {
				names = append(names, e.Type)
			}
		}
	}
	for _, path := range strings.Split(samples, ";") {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		captured := int(float64(w) * runePanelWidths[len(runePanelWidths)-1])
		rgba := image.NewRGBA(image.Rect(0, 0, captured, h))
		draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
		pixels := make([]byte, len(rgba.Pix))
		for i := 0; i < len(pixels); i += 4 {
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = rgba.Pix[i+2], rgba.Pix[i+1], rgba.Pix[i], 255
		}
		start := time.Now()
		rows, err := readRunePanel(pixels, captured, w, h, names)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d rows in %v", filepath.Base(path), len(rows), time.Since(start).Round(time.Millisecond))
		for _, r := range rows {
			t.Logf("  %2dx %-5v %-32q right %4.0f  (%s)", r.Count, r.CountRead, r.Name, r.Right, r.Text)
		}
	}
}
