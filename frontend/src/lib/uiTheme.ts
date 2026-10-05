import { writable } from 'svelte/store'
import { Events } from '@wailsio/runtime'
import { AppService } from '../../bindings/poe2filter/internal/app'
import presets from '../../../internal/uitheme/presets.json'
import { applyThemePalette, paletteKeys } from '../../public/ui-theme.mjs'

export type UITheme = { id: string; name: string; base: string; colors: Record<string,string>; grain: boolean; readOnly: boolean }
export type UIThemeState = { version: number; revision: number; selected: string; themes: UITheme[] }
export const themeFields = paletteKeys
const initial: UIThemeState = { version:1, revision:0, selected:'default', themes:presets }
export const uiThemes = writable<UIThemeState>(initial)
let state = initial
const cacheKey = 'mrw-ui-theme-v1'
const hex = /^#[\da-f]{6}$/i
const listeners = new Set<(payload: ThemePayload)=>void>()
export type ThemePayload = { type:'craft-theme'; base:string; colors:Record<string,string>; reference:Record<string,string>; grain:boolean }

export function themePayload(): ThemePayload {
  const t=state.themes.find(t=>t.id===state.selected) ?? presets[0]
  const base=presets.find(p=>p.id===t.base)??presets[0]
  return { type:'craft-theme',base:t.base,colors:{...t.colors},reference:{...base.colors},grain:t.grain }
}
export function onThemeChange(fn:(payload:ThemePayload)=>void) { listeners.add(fn); return ()=>{listeners.delete(fn)} }
export function acceptUIThemes(value: unknown): UIThemeState|null {
  const next=value as UIThemeState|null
  if (!next || next.version!==1 || !Array.isArray(next.themes) || next.revision<state.revision) return null
  const selected=next.themes.find(t=>t.id===next.selected)
  if (!selected || !presets.some(t=>t.id===selected.base) || !themeFields.every(k=>hex.test(selected.colors?.[k]))) return null
  state=next; uiThemes.set(next)
  applyThemePalette(themePayload())
  document.documentElement.dataset.uiThemeId=selected.id
  try { localStorage.setItem(cacheKey,JSON.stringify(next)) } catch { /* appearance still applies */ }
  for(const fn of listeners)fn(themePayload())
  return next
}
export function followUITheme() {
  // Cached appearance is only for first paint; disk state remains authoritative.
  try {
    const cached=JSON.parse(localStorage.getItem(cacheKey)??'null') as UIThemeState|null
    if(cached) { cached.revision=0; acceptUIThemes(cached) }
  } catch { /* missing or invalid cache */ }
  const off=Events.On('ui-theme',event=>{acceptUIThemes(event.data)})
  // A hidden window may miss a runtime event while reconnecting. Refresh when
  // it becomes usable again; revision checks discard older in-flight replies.
  const refresh=()=>{void AppService.GetUIThemes().then(acceptUIThemes).catch(()=>{})}
  const visible=()=>{if(document.visibilityState==='visible')refresh()}
  window.addEventListener('focus',refresh)
  document.addEventListener('visibilitychange',visible)
  refresh()
  return ()=>{off();window.removeEventListener('focus',refresh);document.removeEventListener('visibilitychange',visible)}
}
