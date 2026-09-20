// Package i18n holds the user-facing texts the Go side produces: status steps,
// log lines, tray menu, notifications, style group names and the comments in
// the generated filter. Internal error strings stay English and are not keyed
// here; they are diagnostics, not interface texts.
package i18n

import (
	"fmt"
	"sync"
)

// Lang is a supported interface language.
type Lang string

const (
	TR Lang = "tr"
	EN Lang = "en"
	ZH Lang = "zh-Hant"
	// Auto asks for the Windows display language, resolved by Resolve.
	Auto Lang = "auto"
)

// Supported lists the languages a user can pick, in menu order.
var Supported = []Lang{EN, TR, ZH}

// Names are the language names, each written in its own language.
var Names = map[Lang]string{EN: "English", TR: "Türkçe", ZH: "繁體中文"}

var (
	mu      sync.RWMutex
	current = EN
)

// Set switches the active language. Unknown values fall back to English.
func Set(l Lang) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := tables[l]; !ok {
		l = EN
	}
	current = l
}

// Current reports the active language.
func Current() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Resolve turns a config value ("auto", "tr", …) into a concrete language.
func Resolve(setting string) Lang {
	l := Lang(setting)
	if l == Auto || l == "" {
		return detect()
	}
	if _, ok := tables[l]; !ok {
		return EN
	}
	return l
}

// Valid reports whether a config value is one we accept.
func Valid(setting string) bool {
	if setting == string(Auto) {
		return true
	}
	_, ok := tables[Lang(setting)]
	return ok
}

// T returns the text for key in the active language, formatted with args.
// A missing key falls back to English and then to the key itself, so a
// forgotten translation shows up as readable text rather than an empty label.
func T(key string, args ...any) string {
	return In(Current(), key, args...)
}

// In is T for a specific language.
func In(l Lang, key string, args ...any) string {
	s, ok := tables[l][key]
	if !ok {
		if s, ok = tables[EN][key]; !ok {
			s = key
		}
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}
