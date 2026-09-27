package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The browser refuses to load the extension when a file the manifest names
// is missing ("Could not load icon 'icons/icon-16.png'"), which happened
// when the copy flattened the icons/ folder.
func TestPreparedExtensionHasEveryFileItsManifestNames(t *testing.T) {
	dir := t.TempDir()
	// Leftovers of the old flattened copy must go.
	os.MkdirAll(filepath.Join(dir, "browser-extension"), 0o755)
	os.WriteFile(filepath.Join(dir, "browser-extension", "icon-16.png"), []byte("old"), 0o644)

	s := &AppService{meta: Meta{DataDir: dir}}
	out, err := s.PrepareBrowserExtension()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Icons  map[string]string `json:"icons"`
		Action struct {
			Icons map[string]string `json:"default_icon"`
		} `json:"action"`
		Background struct {
			ServiceWorker string   `json:"service_worker"`
			Scripts       []string `json:"scripts"`
		} `json:"background"`
		Content []struct {
			JS []string `json:"js"`
		} `json:"content_scripts"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, f := range m.Icons {
		files = append(files, f)
	}
	for _, f := range m.Action.Icons {
		files = append(files, f)
	}
	files = append(files, m.Background.ServiceWorker)
	files = append(files, m.Background.Scripts...)
	for _, c := range m.Content {
		files = append(files, c.JS...)
	}
	if len(m.Icons) == 0 {
		t.Fatal("manifest names no icons; the test would prove nothing")
	}
	for _, f := range files {
		if f == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Errorf("manifest names %s but it is not in the prepared folder", f)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "icon-16.png")); err == nil {
		t.Error("flattened leftover icon-16.png was not removed")
	}
}
