package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The version lives in three places that have to agree: the string the panel
// shows, the Wails build config, and the Windows version resource that ends up
// in the exe's file properties. Forgetting one of them during a release ships a
// binary whose properties disagree with its own about line, and the metadata is
// also a condition for code signing, so guard it with a test.
func TestVersionIsConsistent(t *testing.T) {
	cfg, err := os.ReadFile("build/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	semver := regexp.MustCompile(`version:\s*"(\d+\.\d+\.\d+)"`)
	m := semver.FindSubmatch(cfg)
	if m == nil {
		t.Fatal("build/config.yml: no info.version found")
	}
	if got := string(m[1]); got != version {
		t.Errorf("build/config.yml has %q, main.version is %q", got, version)
	}

	raw, err := os.ReadFile("build/windows/info.json")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Fixed struct {
			FileVersion    string `json:"file_version"`
			ProductVersion string `json:"product_version"`
		} `json:"fixed"`
		Info map[string]map[string]string `json:"info"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	// The fixed block wants four parts, the string block the plain version.
	for name, got := range map[string]string{
		"fixed.file_version":    info.Fixed.FileVersion,
		"fixed.product_version": info.Fixed.ProductVersion,
	} {
		if got != version+".0" {
			t.Errorf("info.json %s = %q, want %q", name, got, version+".0")
		}
	}

	// Windows only resolves the string block when its key matches the language
	// in VarFileInfo\Translation; the generator derives both from this key, and
	// "0409" (en-US) is the one it turns into a readable "040904b0" block.
	strs, ok := info.Info["0409"]
	if !ok {
		t.Fatalf("info.json: expected a %q language block, got %v", "0409", keysOf(info.Info))
	}
	if got := strs["ProductVersion"]; got != version {
		t.Errorf("info.json ProductVersion = %q, want %q", got, version)
	}
	for _, key := range []string{"ProductName", "CompanyName", "FileDescription"} {
		if v := strings.TrimSpace(strs[key]); v == "" || strings.Contains(v, "My ") {
			t.Errorf("info.json %s = %q: still the Wails template placeholder", key, strs[key])
		}
	}
}

func keysOf(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
