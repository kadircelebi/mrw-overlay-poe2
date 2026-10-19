package invscan

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func pngBytes(t *testing.T, c color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < 64; i++ {
		img.SetNRGBA(i%8, i/8, c)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestIconCacheDownloadsOnceAndGroupsSharedIcons(t *testing.T) {
	red, blue := pngBytes(t, color.NRGBA{255, 0, 0, 255}), pngBytes(t, color.NRGBA{0, 0, 255, 255})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "MrW-Overlay/") {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/exalted.png":
			w.Write(red)
		case "/divine.png":
			w.Write(blue)
		case "/broken.png":
			w.Write([]byte("not a picture"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache := IconCache{Dir: dir, Client: srv.Client()}
	sources := []Source{
		{Entry{ID: "exalted", Name: "Exalted Orb"}, srv.URL + "/exalted.png"},
		{Entry{ID: "divine", Name: "Divine Orb"}, srv.URL + "/divine.png"},
		// The three tiers of an orb share one picture.
		{Entry{ID: "greater-exalted-orb", Name: "Greater Exalted Orb"}, srv.URL + "/exalted.png"},
		{Entry{ID: "gone", Name: "Gone"}, srv.URL + "/gone.png"},
		{Entry{ID: "broken", Name: "Broken"}, srv.URL + "/broken.png"},
		{Entry{ID: "none", Name: "No icon"}, ""},
	}
	icons, err := cache.Load(context.Background(), sources)
	if err == nil || !strings.Contains(err.Error(), "2 of 4") {
		t.Errorf("want 2 of 4 failed, got %v", err)
	}
	if len(icons) != 2 || icons[0].Entries[0].Name != "Exalted Orb" || len(icons[0].Entries) != 2 || icons[1].Entries[0].Name != "Divine Orb" {
		t.Fatalf("icons: %+v", icons)
	}
	if c := color.NRGBAModel.Convert(icons[1].Image.At(0, 0)).(color.NRGBA); c.B != 255 {
		t.Errorf("Divine Orb's picture: %v", c)
	}
	first := hits.Load()
	if first != 4 {
		t.Errorf("%d requests, want one per address (4)", first)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(files) != 2 {
		t.Errorf("%d files kept, want the 2 good ones", len(files))
	}

	// Again: the kept icons come from disk; only the missing ones are asked.
	icons, _ = cache.Load(context.Background(), sources)
	if len(icons) != 2 || hits.Load()-first != 2 {
		t.Errorf("second load: %d icons, %d new requests", len(icons), hits.Load()-first)
	}

	// A file broken on disk is fetched again.
	if err := os.WriteFile(files[0], []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	if icons, _ = cache.Load(context.Background(), sources[:2]); len(icons) != 2 || hits.Load()-before != 1 {
		t.Errorf("broken file: %d icons, %d requests", len(icons), hits.Load()-before)
	}
}
