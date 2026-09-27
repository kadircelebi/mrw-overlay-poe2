package main

import (
	"encoding/json"
	"testing"
)

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
