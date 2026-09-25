<script lang="ts">
  import { onMount } from 'svelte'
  import { AppService } from '../../bindings/poe2filter'
  import type { QuotaStatus, QuotaWindow } from '../../bindings/poe2filter/internal/trade/models'

  // GGG's search windows for this IP, as the last trade response reported
  // them plus our own searches since. Everything on the IP (the trade site,
  // other overlays) counts, so this is what tells who used the quota up.
  let status = $state<QuotaStatus | null>(null)
  let now = $state(Date.now())

  onMount(() => {
    let alive = true
    const load = () => AppService.OverlayQuota().then((s) => { if (alive) status = s }).catch(() => {})
    load()
    const poll = setInterval(() => { if (document.visibilityState === 'visible') load() }, 1500)
    const tick = setInterval(() => (now = Date.now()), 1000)
    return () => { alive = false; clearInterval(poll); clearInterval(tick) }
  })

  const observed = $derived(!!status && new Date(status.observedAt).getFullYear() > 2000)
  const penaltyLeft = $derived(status ? Math.max(0, Math.ceil((Date.parse(status.restrictedUntil) - now) / 1000)) : 0)
  // The window closest to its limit is the one that matters.
  const tightest = $derived.by(() => {
    let best: QuotaWindow | null = null
    for (const w of status?.windows ?? []) {
      if (w.limit > 0 && (!best || w.hits / w.limit > best.hits / best.limit)) best = w
    }
    return best
  })
  const level = $derived(penaltyLeft > 0 ? 'bad' : !tightest ? '' : tightest.hits >= tightest.limit ? 'bad' : tightest.hits >= tightest.allowed ? 'warn' : '')

  function windowName(sec: number): string {
    if (sec >= 3600 && sec % 3600 === 0) return `${sec / 3600} sa`
    if (sec >= 60 && sec % 60 === 0) return `${sec / 60} dk`
    return `${sec} sn`
  }

  function clock(sec: number): string {
    if (sec >= 3600) return `${Math.floor(sec / 3600)}:${String(Math.floor((sec % 3600) / 60)).padStart(2, '0')}:${String(sec % 60).padStart(2, '0')}`
    return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`
  }

  const title = $derived.by(() => {
    if (!status || !observed) return 'GGG arama kotası: henüz bir trade yanıtı alınmadı'
    const lines = ['GGG arama kotası (bu IP, tüm uygulamalar dahil):']
    for (const w of status.windows ?? []) {
      lines.push(`${windowName(w.periodSec)}: ${w.hits}/${w.limit}  · bizim sınırımız ${w.allowed} · aşılırsa ${windowName(w.penaltySec)} ceza`)
    }
    if (penaltyLeft > 0) {
      lines.push('', status.restrictedWindowSec
        ? `Ceza: ${windowName(status.restrictedWindowSec)} penceresi aşıldı, ${clock(penaltyLeft)} kaldı`
        : `Ceza: GGG pencereyi bildirmedi, ${clock(penaltyLeft)} kaldı`)
    }
    lines.push('', `Son GGG yanıtı: ${new Date(status.observedAt).toLocaleTimeString()}`)
    return lines.join('\n')
  })
</script>

<span class="quota {level}" {title}>
  {#if penaltyLeft > 0}
    Ceza {clock(penaltyLeft)}{#if status?.restrictedWindowSec} · {windowName(status.restrictedWindowSec)}{/if}
  {:else if observed && tightest}
    {tightest.hits}/{tightest.limit} · {windowName(tightest.periodSec)}
  {:else}
    Kota —
  {/if}
</span>

<style>
  .quota { --wails-draggable: no-drag; flex: 0 0 auto; margin-left: auto; padding: 2px 6px; border: 1px solid #3b3d36; border-radius: 2px; background: #1a1c19; color: var(--muted); font-size: 10px; white-space: nowrap; cursor: help; font-variant-numeric: tabular-nums; }
  .quota.warn { border-color: #6b5a2c; color: var(--warn); }
  .quota.bad { border-color: #6e3230; background: rgba(90, 20, 20, .35); color: #ec8f87; }
</style>
