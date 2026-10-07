//go:build windows

package overlay

import (
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
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
