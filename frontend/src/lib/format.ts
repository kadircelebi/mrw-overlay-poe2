export function clock(ms: number): string {
  if (!ms) return '—'
  return new Date(ms).toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' })
}

export function relative(ms: number, now: number): string {
  if (!ms) return 'hiç'
  const s = Math.round((now - ms) / 1000)
  if (s < 45) return 'az önce'
  const m = Math.round(s / 60)
  if (m < 60) return `${m} dk önce`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} sa ${m % 60} dk önce`
  return new Date(ms).toLocaleDateString('tr-TR', { day: 'numeric', month: 'short' })
}

export function until(ms: number, now: number): string {
  const s = Math.max(0, Math.round((ms - now) / 1000))
  if (s < 60) return `${s} sn`
  const m = Math.round(s / 60)
  if (m < 60) return `${m} dk`
  return `${Math.floor(m / 60)} sa ${m % 60} dk`
}

/** Formats an Exalted amount, switching to Divine when it reads better. */
export function money(ex: number, divineEx: number): string {
  if (divineEx > 0 && ex >= divineEx) {
    const d = ex / divineEx
    return `${d >= 10 ? d.toFixed(0) : d.toFixed(1)} div`
  }
  return `${ex >= 10 ? ex.toFixed(0) : ex.toFixed(1)} ex`
}

export const strictnessNames = [
  'Soft', 'Regular', 'Semi-Strict', 'Strict', 'Very Strict', 'Uber Strict', 'Uber Plus Strict',
]
