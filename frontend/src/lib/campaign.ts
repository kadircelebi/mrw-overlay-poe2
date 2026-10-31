// The campaign guide's words: its notes come in English and Turkish, the
// other languages read the English.
import { locale } from './i18n.svelte'

export function say(text: { en: string; tr: string }): string {
  return locale() === 'tr-TR' ? text.tr : text.en
}

const MARKS: Record<string, string> = { must: '★', optional: '○', tip: '·', next: '→' }

export function noteMark(kind: string): string {
  return MARKS[kind] ?? '·'
}
