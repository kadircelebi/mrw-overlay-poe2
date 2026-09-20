// Interface texts for the panel. The Go side has its own table for what it
// produces (status steps, tray, notifications); this one covers the UI chrome.
//
// Item, currency and filter keywords stay in English in every language: the
// filter matches items by their English names, so anything the user may type
// into a list keeps that spelling.
import { en } from './locales/en'
import { tr } from './locales/tr'
import { zh } from './locales/zh'

export type Lang = 'en' | 'tr' | 'zh-Hant'

const tables: Record<Lang, Record<string, string>> = { en, tr, 'zh-Hant': zh }

let lang = $state<Lang>('en')

/** Switches the active language; unknown values fall back to English. */
export function setLang(l: string) {
  lang = (l in tables ? l : 'en') as Lang
}

export function currentLang(): Lang {
  return lang
}

/** BCP-47 tag for Intl formatting (dates, clock). */
export function locale(): string {
  return lang === 'tr' ? 'tr-TR' : lang === 'zh-Hant' ? 'zh-TW' : 'en-GB'
}

/**
 * Text for a key, with {0}, {1}… replaced by args. A missing key falls back to
 * English and then to the key itself, so a gap shows up as readable text.
 */
export function t(key: string, ...args: (string | number)[]): string {
  const s = tables[lang][key] ?? en[key] ?? key
  return args.length ? s.replace(/\{(\d+)\}/g, (m, i) => String(args[Number(i)] ?? m)) : s
}
