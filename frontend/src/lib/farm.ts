// The farm's figures as the tracker window and the loot box show them: the
// lines a player can put in the box, each with its label and value. Times
// come from the game's log as they stood when sent; while the player is in
// a map or a counted stay at home they run on from there.
import type { FarmView, InvScanMap } from '../../bindings/poe2filter/internal/app/models'
import type { Doubt, Total } from '../../bindings/poe2filter/internal/lootlog/models'
import { t } from './i18n.svelte'
import { money } from './format'

export type FarmData = {
  farm: FarmView
  session: Total
  divineEx: number
  lastRun: InvScanMap | null
  // What went into the session (0: not told).
  costEx: number
}

// Lines only the box shows (the scan's state, the live clock, the
// inventory); the rest are the tracker's figures too.
export const BOX_ONLY = ['status', 'timer', 'inventory']
export const FIGURES = ['sessionTime', 'sessionGain', 'sessionNet', 'maps', 'perMap', 'perHour', 'perMapHour', 'avgTime', 'mapTime', 'restTime', 'lastTime', 'lastGain', 'best']

/** h:mm:ss, or m:ss under an hour. */
export function duration(secs: number): string {
  if (!(secs > 0)) return '—'
  const s = Math.floor(secs)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

/** The times as they stand at now. */
export function liveTimes(f: FarmView, now: number) {
  const dt = f.live ? Math.max(0, (now - f.at) / 1000) : 0
  const map = f.live === 'map' ? dt : 0
  const rest = f.live === 'rest' ? Math.min(dt, f.restLeft) : 0
  return {
    active: f.active + map + rest,
    inMaps: f.inMaps + map,
    inRest: f.inRest + rest,
    breaks: f.breaks + dt - map - rest,
    // How long the current map trip or stay has lasted.
    stay: f.live && f.since ? Math.max(0, (now - f.since) / 1000) : 0,
  }
}

export function fieldLabel(key: string, d?: FarmData): string {
  if (key === 'lastGain' && d?.lastRun) return `${t('farm.lastGain')} · ${d.lastRun.name}`
  if (key === 'lastTime' && d?.farm.lastMap) return `${t('farm.lastTime')} · ${d.farm.lastMap}`
  if (key === 'best' && d?.farm.best) return `${t('farm.best')} · ${d.farm.best.name}`
  return t(`farm.${key}`)
}

/** A figure's value at now ('—' when not known yet). */
export function fieldValue(key: string, d: FarmData, now: number): string {
  const f = d.farm
  const times = liveTimes(f, now)
  const worth = (ex: number) => money(ex, d.divineEx)
  const gain = d.session.valueEx
  const hours = (secs: number) => secs / 3600
  switch (key) {
    case 'sessionTime':
      return f.timed ? duration(times.active) : '—'
    case 'sessionGain':
      return worth(gain)
    case 'sessionNet': {
      const net = gain - d.costEx
      return net < 0 ? `-${worth(-net)}` : worth(net)
    }
    case 'maps':
      return String(d.session.maps)
    case 'perMap':
      return d.session.maps > 0 ? worth(gain / d.session.maps) : '—'
    case 'perHour':
      return f.timed && times.active >= 60 ? worth(gain / hours(times.active)) : '—'
    case 'perMapHour':
      return f.timed && times.inMaps >= 60 ? worth(gain / hours(times.inMaps)) : '—'
    case 'avgTime':
      return f.maps > 0 ? duration(times.inMaps / f.maps) : '—'
    case 'mapTime':
      return f.timed ? duration(times.inMaps) : '—'
    case 'restTime':
      return f.timed ? duration(times.inRest) : '—'
    case 'lastTime':
      return duration(f.lastSecs)
    case 'lastGain':
      if (!d.lastRun) return '—'
      return d.lastRun.noBase ? '?' : `+${worth(d.lastRun.runEx)}`
    case 'best':
      return f.best ? worth(f.best.valueEx) : '—'
  }
  return ''
}

/** Where a cell not read for sure is, what it was taken for and why. */
export function doubtText(d: Doubt): string {
  const what = d.name ? `${d.name}${d.count > 1 ? ` ×${d.count}` : ''}` : '?'
  return `${t('invscan.cell', d.row, d.col)}: ${what} (${t(`invscan.why.${d.why}`)})`
}

/** The live clock's words: where the player is and for how long. */
export function timerText(f: FarmView, now: number): string {
  const { stay } = liveTimes(f, now)
  switch (f.live) {
    case 'map':
      return `${t('farm.inMapNow')} ${duration(stay)}`
    case 'rest':
      return `${t('farm.inRestNow')} ${duration(stay)}`
    case 'break':
      return t('farm.onBreak')
  }
  return t('farm.noClock')
}
