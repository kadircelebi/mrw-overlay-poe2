package app

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

	// A larger personal scale always means larger windows; one taller than
	// the game is cut to it, and no page grows wider than the game.
	for name, a := range map[string]windowArea{"1440p": qhd, "1080p 125%": cases["1080p 125%"]} {
		prevO, prevC, prevM := 0.0, 0.0, 0.0
		for _, pct := range []int{75, 100, 125, 150, 175} {
			st := overlay.Settings{AutoScale: true, UIScale: pct}
			o, c, m := overlayScaleFor(st, a), craftScaleFor(st, a), marketScaleFor(st, a)
			if o < prevO || c < prevC || m < prevM {
				t.Errorf("%s %d%%: scales went down (%.2f %.2f %.2f after %.2f %.2f %.2f)", name, pct, o, c, m, prevO, prevC, prevM)
			}
			if h := overlayWindowHeight(a, overlayHeight, o); float64(h) > a.height*0.951 {
				t.Errorf("%s %d%%: overlay window %d tall in %.0f", name, pct, h, a.height)
			}
			if overlayWidth*o > a.width || craftWidth*c > a.width+0.5 {
				t.Errorf("%s %d%%: page wider than the game", name, pct)
			}
			prevO, prevC, prevM = o, c, m
		}
	}
	if big, normal := overlayScaleFor(overlay.Settings{AutoScale: true, UIScale: 175}, qhd), overlayScaleFor(auto, qhd); big < normal*1.5 {
		t.Errorf("175%% overlay %.2f is barely larger than 100%% %.2f", big, normal)
	}
	if big := craftScaleFor(overlay.Settings{AutoScale: true, UIScale: 175}, qhd); big < 1.3 {
		t.Errorf("175%% craft %.2f did not grow", big)
	}
	small := overlay.Settings{AutoScale: true, UIScale: 75}
	if got, want := craftScaleFor(small, qhd), 0.75*craftScaleFor(auto, qhd); got < want-0.01 || got > want+0.01 {
		t.Errorf("craft ignores the personal scale: %.2f, want %.2f", got, want)
	}
	if got, want := marketScaleFor(small, qhd), 0.75*marketScaleFor(auto, qhd); got < want-0.01 || got > want+0.01 {
		t.Errorf("market ignores the personal scale: %.2f, want %.2f", got, want)
	}
}

// The market window grows with the UI size so its columns keep their room:
// the page width (window width / zoom) never drops below what it has at the
// default size, and never under the narrowest layout, unless the screen or
// the game is too small for it.
func TestMarketWidthGrowsWithUISize(t *testing.T) {
	screen := screenOf(3840, 2160, 1.5, 48)
	a := areaOf(screen, 3840, 2160, true)
	work := screen.WorkArea.Width
	normal := overlay.Settings{AutoScale: true, UIScale: 100}
	if got := marketWidthFor(normal, a, work); got != work/3 {
		t.Fatalf("default market width %d, want a third (%d)", got, work/3)
	}
	page100 := float64(marketWidthFor(normal, a, work)) / marketScaleFor(normal, a)
	prev := 0
	for _, pct := range []int{75, 100, 125, 150, 175} {
		st := overlay.Settings{AutoScale: true, UIScale: pct}
		w := marketWidthFor(st, a, work)
		if w < prev {
			t.Errorf("%d%%: window narrowed to %d", pct, w)
		}
		prev = w
		if w > work {
			t.Errorf("%d%%: window %d wider than the screen %d", pct, w, work)
		}
		page := float64(w) / marketScaleFor(st, a)
		if w < work && (page < page100-1 || page < marketMinWidth-1) {
			t.Errorf("%d%%: page only %.0f wide (default %.0f)", pct, page, page100)
		}
	}

	// A small game caps the window at the game's width.
	small := areaOf(screenOf(3840, 2160, 1.5, 48), 1280, 720, true)
	if got := marketWidthFor(overlay.Settings{AutoScale: true, UIScale: 175}, small, work); float64(got) > small.width {
		t.Errorf("market %d wider than the game %.0f", got, small.width)
	}
}
