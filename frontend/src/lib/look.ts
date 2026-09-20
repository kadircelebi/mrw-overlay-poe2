import { t } from './i18n.svelte'

import type { StyleGroup, Theme } from '../../bindings/poe2filter/internal/filter/models'

/** What a drop looks like on the ground: colours are "R G B [A]" strings. */
export type Look = { bg: string; text: string; border: string; beam: string; icon: string; shape: string }

/** Mirrors style.with in internal/filter/themes.go so previews match the filter. */
export function lookOf(g: StyleGroup, t: Theme): Look {
  const iconColour = t.icon || (t.beam || '').split(' ')[0]
  if (t.full) {
    return { bg: t.bg, text: t.text, border: t.border, beam: t.beam, shape: t.shape, icon: t.shape ? iconColour : '' }
  }
  return {
    bg: t.bg,
    text: t.text,
    border: t.border,
    beam: g.hasBeam ? t.beam : '',
    shape: g.iconShape ? t.shape || g.iconShape : '',
    icon: g.iconShape ? iconColour : '',
  }
}

/** Filter colour "R G B [A]" (alpha 0..255) to CSS. */
export function css(c: string | undefined): string {
  const [r, g, b, a = 255] = (c ?? '').split(' ').map(Number)
  return Number.isFinite(r) ? `rgba(${r}, ${g}, ${b}, ${a / 255})` : 'transparent'
}

/** Filter colour to "#rrggbb" for colour inputs (fallback when unset). */
export function toHex(c: string | undefined, fallback = '#000000'): string {
  const [r, g, b] = (c ?? '').split(' ').map(Number)
  if (![r, g, b].every(Number.isFinite)) return fallback
  return '#' + [r, g, b].map((v) => Math.max(0, Math.min(255, v)).toString(16).padStart(2, '0')).join('')
}

/** In-game effect / minimap colour names. */
export const effectColours: Record<string, string> = {
  Blue: '#4a7dff', Brown: '#b07040', Cyan: '#2fe6f0', Green: '#3be36a', Grey: '#9a9a9a', Orange: '#ff9a2e',
  Pink: '#ff6fb5', Purple: '#b25cff', Red: '#ff3b3b', White: '#f4f4f4', Yellow: '#ffd23a',
}

export function effectCss(name: string): string {
  return effectColours[(name || '').split(' ')[0]] ?? '#cccccc'
}

/** Minimap icon shapes as 24x24 SVG paths. */
export const shapePaths: Record<string, string> = {
  Star: 'M12 2l2.9 6.6 7.1.6-5.4 4.7 1.6 7L12 17.3 5.8 20.9l1.6-7L2 9.2l7.1-.6z',
  Diamond: 'M12 2l9 10-9 10-9-10z',
  Circle: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z',
  Square: 'M4 4h16v16H4z',
  Triangle: 'M12 3l10 18H2z',
  Hexagon: 'M7 3h10l5 9-5 9H7l-5-9z',
  Pentagon: 'M12 2l10 7.3-3.8 11.7H5.8L2 9.3z',
  Cross: 'M9 2h6v7h7v6h-7v7H9v-7H2V9h7z',
  Kite: 'M12 2l7 8-7 12-7-12z',
  UpsideDownHouse: 'M3 3h18v9l-9 9-9-9z',
}

/** Minimap shape name in the active language. */
export function shapeName(shape: string): string {
  return shape ? t('shape.' + shape) : ''
}

/** Effect/icon colour name in the active language. */
export function colourName(colour: string): string {
  return colour ? t('colour.' + colour) : ''
}

/** "#rrggbb" to a filter colour "R G B 255". */
export function fromHex(h: string): string {
  const n = parseInt(h.slice(1), 16)
  return `${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255} 255`
}
