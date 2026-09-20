import { t, locale } from './i18n.svelte'

export function clock(ms: number): string {
  if (!ms) return '—'
  // 24-hour everywhere: zh-TW would otherwise prefix 上午/下午 and wrap the row.
  return new Date(ms).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit', hour12: false })
}

export function relative(ms: number, now: number): string {
  if (!ms) return t('time.never')
  const s = Math.round((now - ms) / 1000)
  if (s < 45) return t('time.justNow')
  const m = Math.round(s / 60)
  if (m < 60) return t('time.minsAgo', m)
  const h = Math.floor(m / 60)
  if (h < 24) return t('time.hoursAgo', h, m % 60)
  return new Date(ms).toLocaleDateString(locale(), { day: 'numeric', month: 'short' })
}

export function until(ms: number, now: number): string {
  const s = Math.max(0, Math.round((ms - now) / 1000))
  if (s < 60) return t('time.secs', s)
  const m = Math.round(s / 60)
  if (m < 60) return t('time.mins', m)
  return t('time.hours', Math.floor(m / 60), m % 60)
}

/** Formats an Exalted amount, switching to Divine when it reads better. */
export function money(ex: number, divineEx: number): string {
  if (divineEx > 0 && ex >= divineEx) {
    const d = ex / divineEx
    return `${d >= 10 ? d.toFixed(0) : d.toFixed(1)} div`
  }
  return `${ex >= 10 ? ex.toFixed(0) : ex.toFixed(1)} ex`
}

// Strictness levels keep NeverSink's own English names in every language.
export const strictnessNames = [
  'Soft', 'Regular', 'Semi-Strict', 'Strict', 'Very Strict', 'Uber Strict', 'Uber Plus Strict',
]
