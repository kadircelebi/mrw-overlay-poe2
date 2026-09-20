package i18n

import (
	"strings"
	"testing"
)

// Every language must cover every English key, and no language may invent a key
// English does not have: a typo in one table would silently fall back forever.
func TestTablesCoverTheSameKeys(t *testing.T) {
	for lang, table := range tables {
		if lang == EN {
			continue
		}
		for key := range en {
			if _, ok := table[key]; !ok {
				t.Errorf("%s: missing key %q", lang, key)
			}
		}
		for key := range table {
			if _, ok := en[key]; !ok {
				t.Errorf("%s: unknown key %q", lang, key)
			}
		}
	}
}

// A translated format string must keep the verbs of the English one, otherwise
// Sprintf prints %!d(MISSING) to the user.
func TestFormatVerbsMatch(t *testing.T) {
	verbs := func(s string) string {
		var out []string
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '%' {
				if s[i+1] == '%' {
					i++
					continue
				}
				j := i + 1
				for j < len(s) && strings.ContainsRune("+-# 0123456789.", rune(s[j])) {
					j++
				}
				if j < len(s) {
					out = append(out, s[i:j+1])
				}
			}
		}
		return strings.Join(out, ",")
	}
	for lang, table := range tables {
		for key, want := range en {
			got, ok := table[key]
			if !ok {
				continue
			}
			if verbs(want) != verbs(got) {
				t.Errorf("%s %q: format verbs %q, English has %q", lang, key, verbs(got), verbs(want))
			}
		}
	}
}

func TestResolveAndDetect(t *testing.T) {
	for _, c := range []struct {
		tag  string
		want Lang
	}{
		{"tr-TR", TR}, {"tr", TR}, {"zh-TW", ZH}, {"zh-Hant-HK", ZH},
		{"zh-CN", EN}, {"en-US", EN}, {"de-DE", EN}, {"", EN},
	} {
		if got := fromTag(c.tag); got != c.want {
			t.Errorf("fromTag(%q) = %q, want %q", c.tag, got, c.want)
		}
	}
	if got := Resolve("zh-Hant"); got != ZH {
		t.Errorf("Resolve(zh-Hant) = %q", got)
	}
	if got := Resolve("klingon"); got != EN {
		t.Errorf("unknown language should fall back to English, got %q", got)
	}
	if !Valid("auto") || !Valid("tr") || Valid("de") {
		t.Error("Valid does not accept exactly auto plus the supported languages")
	}
}

func TestMissingKeyFallsBackToEnglish(t *testing.T) {
	Set(TR)
	defer Set(EN)
	if got := T("app.description"); got != tr["app.description"] {
		t.Errorf("Turkish text expected, got %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key should return itself, got %q", got)
	}
}
