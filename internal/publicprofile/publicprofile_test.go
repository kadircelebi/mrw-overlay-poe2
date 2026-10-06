package publicprofile

import (
	"encoding/json"
	"strings"
	"testing"

	"poe2filter/internal/filter"
)

func authorConfig() filter.Config {
	c := filter.DefaultConfig()
	c.LeagueName = "Author League"
	c.FilterName = "author_filter"
	c.CustomBaseFilter = `C:\Users\author\secret.filter`
	c.SharedScan, c.ExceptionalScan = false, true
	c.MinValue, c.MinValueUnit, c.Strictness = 7, "chaos", 5
	c.WaystoneTier = 12
	c.ItemGroups = []filter.ItemGroup{{ID: "g1", Name: "Expedition", Items: []string{"Expedition Logbook"}}}
	c.Whitelist = []string{"Divine Orb"}
	c.HiddenItems = []filter.HiddenItem{{Base: "Gold Ring"}}
	c.Sounds = map[string]string{filter.GroupDivine: filter.SoundFilePrefix + "divine.mp3"}
	c.Normalize()
	return c
}

func TestRoundTripKeepsFollowersOwnSettings(t *testing.T) {
	doc, swaps := FromConfig(authorConfig())
	if len(swaps) != 1 || swaps[0].Group != filter.GroupDivine || swaps[0].File != "divine.mp3" {
		t.Fatalf("sound swap not reported: %+v", swaps)
	}
	data, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"Author League", "author_filter", "secret.filter", "divine.mp3", "league_name", "exceptional_scan"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("%q leaked into the shared document:\n%s", private, data)
		}
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	local := filter.DefaultConfig()
	local.LeagueName, local.FilterName, local.SharedScan, local.ExceptionalScan = "My League", "mine", true, false
	local.Sounds = map[string]string{filter.GroupDivine: filter.SoundFilePrefix + "mine.mp3"}
	c := got.Apply(local)
	if c.LeagueName != "My League" || c.FilterName != "mine" || !c.SharedScan || c.ExceptionalScan || c.CustomBaseFilter != "" {
		t.Fatalf("follower's own settings were replaced: %+v", c)
	}
	if c.MinValue != 7 || c.MinValueUnit != "chaos" || c.Strictness != 5 || c.WaystoneTier != 12 {
		t.Fatalf("shared settings not applied: %+v", c)
	}
	if len(c.ItemGroups) != 1 || c.ItemGroups[0].Name != "Expedition" || len(c.HiddenItems) != 1 || c.Whitelist[0] != "Divine Orb" {
		t.Fatalf("groups or lists not applied: %+v", c)
	}
	if _, ok := c.Sounds[filter.GroupDivine]; ok {
		t.Fatalf("the author's sound choice should replace the follower's file: %v", c.Sounds)
	}
}

func encodeRaw(t *testing.T, change func(f map[string]any, top map[string]any)) []byte {
	t.Helper()
	doc, _ := FromConfig(authorConfig())
	data, _ := json.Marshal(doc)
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	change(top["filter"].(map[string]any), top)
	out, _ := json.Marshal(top)
	return out
}

func TestDecodeRefusesTamperedDocuments(t *testing.T) {
	cases := map[string][]byte{
		"unknown top field": encodeRaw(t, func(f, top map[string]any) { top["script"] = "<script>alert(1)</script>" }),
		"price source":      encodeRaw(t, func(f, top map[string]any) { f["price_source_url"] = "https://evil.example/prices.json" }),
		"base filter path":  encodeRaw(t, func(f, top map[string]any) { f["custom_base_filter"] = `C:\Windows\win.ini` }),
		"unknown nested field": encodeRaw(t, func(f, top map[string]any) {
			f["item_groups"] = []any{map[string]any{"id": "g1", "name": "x", "items": []any{}, "hide": false, "always": false, "run": "calc.exe"}}
		}),
		"newline in list": encodeRaw(t, func(f, top map[string]any) { f["whitelist"] = []any{"Divine Orb\nHide"} }),
		"newline in group": encodeRaw(t, func(f, top map[string]any) {
			f["item_groups"] = []any{map[string]any{"id": "g1", "name": "a\nHide", "items": []any{}, "hide": false, "always": false}}
		}),
		"huge number":     encodeRaw(t, func(f, top map[string]any) { f["min_value"] = 1e300 }),
		"negative number": encodeRaw(t, func(f, top map[string]any) { f["min_value"] = -1 }),
		"wrong format":    encodeRaw(t, func(f, top map[string]any) { top["format"] = 99 }),
		"long list entry": encodeRaw(t, func(f, top map[string]any) { f["whitelist"] = []any{strings.Repeat("a", MaxTextLen+1)} }),
		"wrong type":      encodeRaw(t, func(f, top map[string]any) { f["whitelist"] = "Divine Orb" }),
		"trailing data":   append(encodeRaw(t, func(f, top map[string]any) {}), []byte(` {"format":1}`)...),
		"invalid utf8":    append([]byte(`{"format":1,"filter":{"whitelist":["`), 0xff, '"', ']', '}', '}'),
		"oversized":       []byte(`{"format":1,"filter":{"whitelist":["` + strings.Repeat("a", MaxBytes) + `"]}}`),
	}
	groups := make([]any, filter.MaxItemGroups+1)
	for i := range groups {
		groups[i] = map[string]any{"id": "g", "name": "x", "items": []any{}, "hide": false, "always": false}
	}
	cases["too many groups"] = encodeRaw(t, func(f, top map[string]any) { f["item_groups"] = groups })

	for name, data := range cases {
		_, err := Decode(data)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		t.Logf("%s: %v", name, err)
	}
	if _, err := Decode(encodeRaw(t, func(f, top map[string]any) {})); err != nil {
		t.Fatalf("an untouched document was refused: %v", err)
	}
}

// What is stored is rebuilt from parsed values, so values the app would not
// keep (an unknown style group, a sound file) never reach a follower.
func TestEncodeIsCanonical(t *testing.T) {
	data := encodeRaw(t, func(f, top map[string]any) {
		f["styles"] = map[string]any{"no-such-group": "x"}
		f["sounds"] = map[string]any{filter.GroupDivine: filter.SoundFilePrefix + "evil.mp3"}
	})
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "no-such-group") || strings.Contains(string(out), "evil.mp3") {
		t.Fatalf("unknown values survived: %s", out)
	}
	again, _ := Decode(out)
	out2, _ := Encode(again)
	if string(out) != string(out2) {
		t.Fatal("encoding is not stable")
	}
}

func TestMeta(t *testing.T) {
	m, err := DecodeMeta([]byte(`{"name":"  Expedition farm ","description":"","tags":["ssf","expedition","ssf"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Expedition farm" || strings.Join(m.Tags, ",") != "expedition,ssf" {
		t.Fatalf("meta not cleaned: %+v", m)
	}
	bad := []string{
		`{"name":"","description":"","tags":[]}`,
		`{"name":"x","description":"","tags":["my own tag"]}`,
		`{"name":"x","description":"","tags":["general","leveling","mapping","expedition","abyss","delirium"]}`,
		`{"name":"` + strings.Repeat("n", MaxNameLen+1) + `","description":"","tags":[]}`,
		`{"name":"a\nb","description":"","tags":[]}`,
		`{"name":"x","description":"","tags":[],"author":"someone else"}`,
	}
	for _, b := range bad {
		if _, err := DecodeMeta([]byte(b)); err == nil {
			t.Errorf("accepted %s", b)
		}
	}
}
