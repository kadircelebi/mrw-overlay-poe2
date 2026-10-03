package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/overlay"
)

func screenOf(physW, physH int, dpi float32, taskbar int) *application.Screen {
	w, h := int(float32(physW)/dpi), int(float32(physH)/dpi)
	return &application.Screen{
		ScaleFactor:    dpi,
		PhysicalBounds: application.Rect{Width: physW, Height: physH},
		WorkArea:       application.Rect{Width: w, Height: h - taskbar},
	}
}

// The UI size covers every in-game window: they keep their size on the
// 1440p/150% screens they were tuned on, shrink to fit a 1080p game, and the
// craft page keeps its two-column width inside a small game window.
func TestWindowScaleFitsTheGame(t *testing.T) {
	auto := overlay.Settings{AutoScale: true, UIScale: 100}

	qhd := areaOf(screenOf(2560, 1440, 1.5, 32), 2560, 1440, true)
	if got := overlayScaleFor(auto, qhd); got < 1.0 {
		t.Errorf("1440p overlay scale %.2f shrank too much", got)
	}
	if got := craftScaleFor(auto, qhd); got < 0.99 {
		t.Errorf("1440p craft scale %.2f, want ~1", got)
	}

	cases := map[string]windowArea{
		"1080p 100%":          areaOf(screenOf(1920, 1080, 1, 40), 1920, 1080, true),
		"1080p 125%":          areaOf(screenOf(1920, 1080, 1.25, 40), 1920, 1080, true),
		"1080p 125% windowed": areaOf(screenOf(1920, 1080, 1.25, 40), 1500, 1200, true),
		"1080p game on 1440p": areaOf(screenOf(2560, 1440, 1.5, 32), 1920, 1080, true),
	}
	for name, a := range cases {
		if a.physicalHeight > 1080 && name != "1080p 125% windowed" {
			t.Errorf("%s: game height %d", name, a.physicalHeight)
		}
		o := overlayScaleFor(auto, a)
		if overlayHeight*o > a.height*0.851 {
			t.Errorf("%s: overlay %.0f tall in %.0f", name, overlayHeight*o, a.height)
		}
		c := craftScaleFor(auto, a)
		if craftHeight*c > a.height+0.5 || craftWidth*c > a.width+0.5 {
			t.Errorf("%s: craft %.0fx%.0f in %.0fx%.0f", name, craftWidth*c, craftHeight*c, a.width, a.height)
		}
	}

	// The personal scale applies to every window, yet never pushes one out of
	// the game.
	fhd := cases["1080p 125%"]
	if got := overlayScaleFor(overlay.Settings{UIScale: 175}, fhd); overlayHeight*got > fhd.height*0.951 {
		t.Errorf("175%% overlay %.0f tall in %.0f", overlayHeight*got, fhd.height)
	}
	if got := craftScaleFor(overlay.Settings{UIScale: 175}, fhd); craftHeight*got > fhd.height+0.5 {
		t.Errorf("175%% craft %.0f tall in %.0f", craftHeight*got, fhd.height)
	}
	small := overlay.Settings{AutoScale: true, UIScale: 75}
	if got, want := craftScaleFor(small, qhd), 0.75*craftScaleFor(auto, qhd); got < want-0.01 || got > want+0.01 {
		t.Errorf("craft ignores the personal scale: %.2f, want %.2f", got, want)
	}
	if got, want := marketScaleFor(small, qhd), 0.75*marketScaleFor(auto, qhd); got < want-0.01 || got > want+0.01 {
		t.Errorf("market ignores the personal scale: %.2f, want %.2f", got, want)
	}
}
