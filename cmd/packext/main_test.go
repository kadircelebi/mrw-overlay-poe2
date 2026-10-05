package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtensionPackageContainsBrowserAssetsOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "icons"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.json", "background.js", "content.js", "embed.go", ".private", "icons/icon-16.png"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := sourceFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"background.js", "content.js", "icons/icon-16.png"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("extension assets = %v, want %v", files, want)
	}
}

func TestStoreManifestsDropTheDevKeyAndForeignSettings(t *testing.T) {
	src := map[string]any{
		"name": "x", "version": "1.0.0", "key": "secret-dev-key",
		"background":                map[string]any{"service_worker": "background.js", "scripts": []any{"background.js"}},
		"browser_specific_settings": map[string]any{"gecko": map[string]any{"id": "a@b"}},
	}
	for target, want := range map[string]struct {
		worker, scripts, gecko bool
	}{
		"chrome":  {worker: true},
		"firefox": {scripts: true, gecko: true},
	} {
		raw, err := ForStore(src, target)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(raw, &m)
		bg := m["background"].(map[string]any)
		_, key := m["key"]
		_, worker := bg["service_worker"]
		_, scripts := bg["scripts"]
		_, gecko := m["browser_specific_settings"]
		if key || worker != want.worker || scripts != want.scripts || gecko != want.gecko {
			t.Errorf("%s manifest = %s", target, raw)
		}
	}
	if _, ok := src["key"]; !ok {
		t.Error("ForStore changed its input")
	}
}
