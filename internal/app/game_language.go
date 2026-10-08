package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/filter"
	"poe2filter/internal/overlay"
	"poe2filter/internal/trade"
)

// setupGameLanguages connects the game language tables to the item reader
// (the trade catalog in that language, fetched once a day like the English
// one) and to the trade results (listings fetched in the item's language).
func (s *AppService) setupGameLanguages() {
	var mu sync.Mutex
	stores := map[string]*overlay.CatalogStore{}
	overlay.LocaleCatalog = func(lang string) (overlay.Catalog, error) {
		mu.Lock()
		store := stores[lang]
		if store == nil {
			store = overlay.NewLocaleCatalogStore(s.meta.DataDir, lang)
			stores[lang] = store
		}
		mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return store.Load(ctx)
	}
	for _, l := range overlay.Locales() {
		// pathofexile.tw is its own realm: a search made on the international
		// site cannot be fetched there, so those listings stay in English.
		if l.Lang == "zh-Hant" {
			continue
		}
		trade.FetchHosts[l.Lang] = overlay.TradeHosts[l.Lang]
		trade.PropertyNames[l.Lang] = l.PropertyNames()
	}
	// The game's language is known from its config before any item is
	// copied: its catalog (a download when missing or a day old) and lookup
	// tables are made ready now, not on the first price check.
	go func() {
		loc := overlay.LocaleByLang(gameLanguage())
		if loc == nil {
			return
		}
		local, err := overlay.LocaleCatalog(loc.Lang)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		english, err := s.overlayCatalog.Load(ctx)
		if err != nil {
			return
		}
		loc.Warm(local, english)
	}()
}

// searchLang remembers the game language of a search, so its next pages
// come in the same language.
func (s *AppService) rememberSearchLang(searchID, lang string) {
	if searchID != "" && lang != "" {
		s.searchLangs.Store(searchID, lang)
	}
}

func (s *AppService) searchLang(searchID string) string {
	if v, ok := s.searchLangs.Load(searchID); ok {
		return v.(string)
	}
	return ""
}

// gameLanguage is the language the game runs in, from its config file
// (Documents\My Games\Path of Exile 2\poe2_production_Config.ini, [LANGUAGE]
// language=…); "" for English or when it cannot be read. Item text the game
// copies says its language itself; this is for what is read off the screen.
func gameLanguage() string {
	dir := filter.GetPoE2GameDir()
	if dir == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(dir, "poe2_production_Config.ini"))
	if err != nil {
		return ""
	}
	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(line)
			continue
		}
		if section != "[language]" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "language" {
			return gameLanguageCode(value)
		}
	}
	return ""
}

// gameLanguageCode turns the config's language value into ours ("" English).
func gameLanguageCode(value string) string {
	v := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	switch {
	case v == "" || strings.HasPrefix(v, "en"):
		return ""
	case strings.HasPrefix(v, "pt") || v == "br":
		return "pt"
	case strings.HasPrefix(v, "zh") || v == "tw" || v == "cht":
		return "zh-Hant"
	}
	if i := strings.IndexByte(v, '-'); i > 0 {
		v = v[:i]
	}
	return v
}

// screenLocale is the game's language tables for reading the screen (nil for
// English), with the recognizer set to that language.
func screenLocale() *overlay.Locale {
	loc := overlay.LocaleByLang(gameLanguage())
	if loc == nil {
		overlay.SetOCRLanguage("")
		return nil
	}
	overlay.SetOCRLanguage(overlay.OCRTags[loc.Lang])
	return loc
}

// englishScreenItem writes the English name into item text read off the
// screen in another language (the line under "Rarity:"), and returns the
// name as the game wrote it ("" when it was already English).
func englishScreenItem(loc *overlay.Locale, text string) (string, string) {
	lines := strings.Split(text, "\n")
	shown := ""
	for i := 0; i+1 < len(lines); i++ {
		if strings.HasPrefix(lines[i], "Rarity:") {
			if en, ok := loc.EnglishName(lines[i+1]); ok {
				shown = lines[i+1]
				lines[i+1] = en
			}
			break
		}
	}
	return strings.Join(lines, "\n"), shown
}
