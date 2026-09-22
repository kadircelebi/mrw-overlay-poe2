package appupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.8.0", "1.7.0", 1},
		{"v1.7.0", "1.7.0", 0},
		{"1.7.0", "1.8.0", -1},
		{"1.10.0", "1.9.9", 1},
		{"not-a-version", "1.0.0", 0},
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCheckAndVerifiedDownload(t *testing.T) {
	payload := []byte("a pretend windows executable")
	digest := sha256.Sum256(payload)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v1.8.0",
				HTMLURL: "https://github.com/kadircelebi/poe2-filtre/releases/tag/v1.8.0",
				Assets: []githubAsset{{
					Name:               releaseAssetName,
					Size:               int64(len(payload)),
					Digest:             "sha256:" + hex.EncodeToString(digest[:]),
					BrowserDownloadURL: server.URL + "/asset",
				}},
			})
		case "/asset":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "poe2filter.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Options{
		CurrentVersion: "1.7.0",
		DataDir:        dir,
		APIURL:         server.URL + "/latest",
		Client:         server.Client(),
		ExecutablePath: exe,
	})
	st, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "available" || st.LatestVersion != "1.8.0" {
		t.Fatalf("unexpected check state: %+v", st)
	}
	st, err = m.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "ready" || st.Progress != 100 {
		t.Fatalf("unexpected download state: %+v", st)
	}
	downloaded, err := os.ReadFile(filepath.Join(dir, "updates", "v1.8.0", releaseAssetName))
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(payload) {
		t.Fatal("downloaded payload changed")
	}

	// A new manager restores the cached result instead of forgetting the known
	// update during the daily no-request window.
	cached := New(Options{CurrentVersion: "1.7.0", DataDir: dir, ExecutablePath: exe})
	if got := cached.State(); got.Status != "ready" || got.LatestVersion != "1.8.0" {
		t.Fatalf("cached state not restored: %+v", got)
	}
}

func TestDownloadRejectsDigestMismatch(t *testing.T) {
	payload := []byte("bad payload")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v2.0.0",
				Assets: []githubAsset{{
					Name:               releaseAssetName,
					Size:               int64(len(payload)),
					Digest:             "sha256:" + strings.Repeat("0", 64),
					BrowserDownloadURL: server.URL + "/asset",
				}},
			})
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dir := t.TempDir()
	m := New(Options{CurrentVersion: "1.7.0", DataDir: dir, APIURL: server.URL + "/latest", Client: server.Client()})
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Download(context.Background()); err == nil {
		t.Fatal("digest mismatch was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "updates", "v2.0.0", releaseAssetName)); !os.IsNotExist(err) {
		t.Fatal("unverified update was kept")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.exe")
	destination := filepath.Join(dir, "destination.exe")
	if err := os.WriteFile(source, []byte("new executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(source, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "new executable" {
		t.Fatalf("copy result = %q, %v", got, err)
	}
}

func TestRuntimeArgsPreserveTestOutput(t *testing.T) {
	got := runtimeArgs(`C:\data`, `C:\scratch\test.filter`, `C:\staged.exe`)
	want := []string{"-data", `C:\data`, "-out", `C:\scratch\test.filter`, "-cleanup-update", `C:\staged.exe`}
	if len(got) != len(want) {
		t.Fatalf("runtimeArgs = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runtimeArgs = %#v", got)
		}
	}
}

func TestWaitForCurrentProcessTimesOut(t *testing.T) {
	if err := waitForProcess(os.Getpid(), 20*time.Millisecond); err == nil {
		t.Fatal("waiting for the current process unexpectedly succeeded")
	}
}

func TestFindReleaseAssetAcceptsLegacyVersionedName(t *testing.T) {
	legacy := "poe2filtre-v1.8.0-windows-amd64.exe"
	got := findReleaseAsset(githubRelease{Assets: []githubAsset{{Name: legacy}}}, "1.8.0")
	if got.Name != legacy {
		t.Fatalf("legacy release asset was not found: %+v", got)
	}
}

func TestFailedRefreshKeepsReadyUpdate(t *testing.T) {
	m := New(Options{CurrentVersion: "1.7.0", DataDir: t.TempDir()})
	m.state = State{Status: "ready", CurrentVersion: "1.7.0", LatestVersion: "1.8.0"}
	m.fallback = m.state
	st, err := m.fail(os.ErrDeadlineExceeded)
	if err == nil || st.Status != "ready" || st.LatestVersion != "1.8.0" || st.Error == "" {
		t.Fatalf("ready fallback was lost: %+v, %v", st, err)
	}
}
