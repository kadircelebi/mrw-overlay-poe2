package gamesounds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func mp3(size int) []byte {
	b := make([]byte, size)
	copy(b, "ID3")
	return b
}

func TestDownloadFetchesOnlyWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir, 2), mp3(8<<10), 0o644); err != nil {
		t.Fatal(err)
	}

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("the tool must identify itself, got %q", r.Header.Get("User-Agent"))
		}
		w.Write(mp3(8 << 10))
	}))
	defer srv.Close()
	defer swapSource(srv.URL + "/AlertSound%d.mp3")()

	got, err := Download(context.Background(), srv.Client(), dir, "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != Count-1 || hits != Count-1 {
		t.Errorf("the sound already on disk should be left alone: downloaded %d, requests %d", got, hits)
	}
	if !Ready(dir) {
		t.Errorf("all sounds should be present now, missing: %v", Missing(dir))
	}

	// A second run has nothing left to do.
	if got, _ := Download(context.Background(), srv.Client(), dir, "test-agent"); got != 0 {
		t.Errorf("a finished set must not be downloaded again, got %d", got)
	}
}

// Cloudflare and captive portals answer with HTML. Writing that out as
// "AlertSound1.mp3" would leave a file that only fails at playback time.
func TestDownloadRejectsNonAudio(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>Just a moment...</body></html>"))
	}))
	defer srv.Close()
	defer swapSource(srv.URL + "/AlertSound%d.mp3")()

	if _, err := Download(context.Background(), srv.Client(), dir, "test-agent"); err == nil {
		t.Fatal("an HTML body should be refused")
	}
	if files, _ := filepath.Glob(filepath.Join(Dir(dir), "*.mp3")); len(files) != 0 {
		t.Errorf("nothing should have been written: %v", files)
	}
}

func TestMissingListsEverythingOnAnEmptyFolder(t *testing.T) {
	if got := Missing(t.TempDir()); len(got) != Count {
		t.Errorf("expected all %d sounds to be missing, got %v", Count, got)
	}
}

func swapSource(u string) func() {
	old := sourceURL
	sourceURL = u
	return func() { sourceURL = old }
}
