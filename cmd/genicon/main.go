// Command genicon draws the app emblem (a gold filter funnel on a dark badge)
// and writes the app icon and tray icons:
//
//	go run ./cmd/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

type pt struct{ x, y float64 }

// Funnel outline in unit coordinates (0..1).
var funnel = []pt{
	{0.20, 0.27}, {0.80, 0.27}, {0.565, 0.53}, {0.565, 0.76},
	{0.435, 0.84}, {0.435, 0.53},
}

func inPoly(p pt, poly []pt) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.y > p.y) != (b.y > p.y) && p.x < (b.x-a.x)*(p.y-a.y)/(b.y-a.y)+a.x {
			in = !in
		}
	}
	return in
}

func lerp(a, b uint8, t float64) float64 { return float64(a) + (float64(b)-float64(a))*t }

// sample returns the colour at unit point p (premultiplied is not needed:
// we average straight RGBA over supersamples and weight by alpha).
func sample(p pt, badge bool) (r, g, b, a float64) {
	dx, dy := p.x-0.5, p.y-0.5
	d := math.Hypot(dx, dy)

	// Gold gradient, lighter at the top.
	t := math.Min(1, math.Max(0, (p.y-0.2)/0.7))
	gold := func() (float64, float64, float64) {
		return lerp(0xF3, 0xB0, t), lerp(0xD9, 0x7E, t), lerp(0x8B, 0x3A, t)
	}

	if inPoly(p, funnel) {
		r, g, b := gold()
		return r, g, b, 255
	}
	if !badge {
		// Tray variant: thin dark outline so the funnel reads on light taskbars.
		for _, o := range []pt{{0.03, 0}, {-0.03, 0}, {0, 0.03}, {0, -0.03}} {
			if inPoly(pt{p.x + o.x, p.y + o.y}, funnel) {
				return 20, 16, 12, 230
			}
		}
		return 0, 0, 0, 0
	}
	switch {
	case d <= 0.43:
		// Dark badge with a subtle radial lift.
		k := 1 - d/0.43
		return 18 + 14*k, 16 + 11*k, 22 + 12*k, 255
	case d <= 0.475:
		r, g, b := gold()
		return r * 0.85, g * 0.85, b * 0.85, 255
	}
	return 0, 0, 0, 0
}

func render(size int, badge bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling per axis
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var sr, sg, sb, sa float64
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					p := pt{(float64(x) + (float64(i)+0.5)/ss) / float64(size), (float64(y) + (float64(j)+0.5)/ss) / float64(size)}
					r, g, b, a := sample(p, badge)
					sr += r * a
					sg += g * a
					sb += b * a
					sa += a
				}
			}
			n := float64(ss * ss)
			if sa == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(sr / sa), uint8(sg / sa), uint8(sb / sa), uint8(sa / n)})
		}
	}
	return img
}

func write(path string, img image.Image) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote", path)
}

func main() {
	write("build/appicon.png", render(1024, true))
	write("internal/assets/tray.png", render(64, false))
	write("frontend/public/emblem.png", render(128, true))
}
