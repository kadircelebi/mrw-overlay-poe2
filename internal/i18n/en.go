package i18n

// tables holds every language. English is the fallback for missing keys, so it
// must define every key the others use.
var tables = map[Lang]map[string]string{EN: en, TR: tr, ZH: zh}

var en = map[string]string{
	// Application
	"app.description": "PoE2 loot filter kept up to date with live market prices",

	// Tray menu and tooltip
	"tray.open":        "Open panel",
	"tray.update":      "Update now",
	"tray.openFolder":  "Open filter folder",
	"tray.quit":        "Quit",
	"tray.updating":    " — updating…",
	"tray.lastFailed":  " — last update failed",
	"tray.lastSuccess": " — last update ",

	// Progress steps
	"step.ready":     "Ready",
	"step.neversink": "Preparing the NeverSink filter",
	"step.prices":    "Fetching market prices",
	"step.rules":     "Generating rules",
	"step.writing":   "Writing the filter",
	"step.done":      "Done",

	// Log lines
	"log.leaguesFailed":     "[Warning] Could not fetch the league list, keeping the current one: %v",
	"log.retry":             "Retrying the failed update (attempt %d)",
	"log.scheduled":         "Scheduled update (every %d hours)",
	"log.scanStopped":       "Exceptional scan stopped.",
	"log.customBaseMissing": "[Warning] Custom base filter not found, using NeverSink.",
	"log.basesFailed":       "[Warning] Could not fetch the exceptional base list: %v",
	"log.written":           "Filter written: %d valuable currency, %d unique bases, %d exceptional",

	// Notification
	"notify.title": "Filter updated",
	"notify.body":  "%s.filter is ready. Use Item Filter → Reload in game.",

	// Errors shown in the panel
	"err.updateRunning":     "an update is already running",
	"err.gameDir":           "Path of Exile 2 folder not found",
	"err.gameDirLong":       "Path of Exile 2 folder not found (Documents\\My Games\\Path of Exile 2)",
	"err.baseRead":          "could not read the base filter (%s): %w",
	"err.filterWrite":       "could not write the filter: %w",
	"err.settingsSave":      "could not save the settings: %w",
	"err.noPriceSource":     "no price source worked: ",
	"err.neversinkDownload": "could not download the NeverSink filter: %w",
	"err.soundInvalid":      "invalid sound file",
	"err.soundNotFound":     "%s is not in the filter folder",
	"err.soundWindowsOnly":  "sound preview only works on Windows",

	// Warnings collected while building rules
	"warn.blacklistSkipped": "Blacklist: %q skipped, the same base (%s) carries a valuable %s",
	"warn.blacklistUnknown": "Blacklist: %q not recognised",

	// Insights categories
	"insights.crafting":       "Crafting base",
	"insights.chanceCrafting": "Chance & crafting base",

	// Theme names
	"theme.neon_cyan":     "Cyan background",
	"theme.neon_purple":   "Purple background",
	"theme.neon_red":      "Red background",
	"theme.neon_gold":     "Gold background",
	"theme.neon_green":    "Green background",
	"theme.dark":          "Dark background, cyan border",
	"theme.gold":          "Dark gold",
	"theme.classic_black": "White background, black border",
	"theme.custom":        "Custom",

	// Style group names
	"group.divine":             "Divine Orb",
	"group.currency":           "Valuable currency",
	"group.whitelist":          "Always show (spotlight)",
	"group.whitelistMid":       "Always show (medium)",
	"group.unique":             "Valuable uniques",
	"group.exceptional":        "Valuable exceptional",
	"group.exceptionalUnknown": "Unpriced exceptional",
	"group.t5rare":             "T5 rare",
	"group.chance":             "Chance bases",

	// Style group defaults
	"groupDefault.currency":           "Default (category colours)",
	"groupDefault.whitelist":          "Default (red, gold border)",
	"groupDefault.whitelistMid":       "Default (purple)",
	"groupDefault.unique":             "Default (dark red)",
	"groupDefault.exceptional":        "Default (dark blue)",
	"groupDefault.exceptionalUnknown": "Default (muted blue)",
	"groupDefault.t5rare":             "Default (dark brown, gold text)",
	"groupDefault.chance":             "Default (grey)",

	// Comments in the generated filter
	"filter.header":        "[[DYNAMIC LOOT FILTER]] - poe2-filter (poe.ninja, poe2scout, trade exceptional scan)",
	"filter.threshold":     "Threshold: %.2f %s (= %.1f Exalted)",
	"filter.rate":          "Exchange Rate: 1 Divine = %.1f Exalted",
	"filter.pricesAt":      "Prices generated: %s",
	"filter.writtenAt":     "Filter written:   %s",
	"filter.lowValue":      "Low-value action: %s",
	"filter.sec.blacklist": "1. USER BLACKLIST (always hidden)",
	"filter.sec.divine":    "2. DIVINE ORB SPOTLIGHT",
	"filter.sec.whitelist": "3. USER WHITELIST (always shown)",
	"filter.sec.currency":  "4. VALUABLE CURRENCY & BULK ITEMS",
	"filter.sec.unique":    "5. VALUABLE UNIQUE BASES (best unique on the base >= threshold)",
	"filter.sec.chance":    "6. CHANCE & CRAFTING BASES",
	"filter.sec.except":    "7. VALUABLE EXCEPTIONAL BASES (trade scan)",
	"filter.sec.t5rare":    "8.1 TIER 5 RARE EQUIPMENT",
	"filter.sec.jewels":    "8.2 RARE JEWELS",
	"filter.sec.quality":   "8.3 HIGH QUALITY GEAR (Quality >= %d)",
	"filter.sec.waystones": "8.4 HIGH-TIER WAYSTONES (T14+)",
	"filter.sec.gems20":    "8.5 UNCUT GEMS (level 20 only)",
	"filter.sec.gems":      "8.5 UNCUT GEMS",
	"filter.sec.pinnacle":  "8.6 PINNACLE KEYS",
	"filter.sec.mid":       "8.7 USER LIST - MEDIUM HIGHLIGHT",
	"filter.sec.below":     "9. BELOW THRESHOLD (%s)",
	"filter.sec.unpriced":  "10. EXCEPTIONAL BASES NOT YET PRICED (shown until scanned)",
}
