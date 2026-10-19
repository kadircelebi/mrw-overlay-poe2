<script lang="ts">
  // The inventory's currency worth, laid over the game left of the
  // inventory panel. The window covers the game's client area and lets
  // clicks through; places come in the game's physical pixels.
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { InvScanView } from '../bindings/poe2filter/internal/app/models'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { money } from './lib/format'
  import { doubtText } from './lib/farm'

  let view = $state<InvScanView | null>(null)
  let dpr = $state(window.devicePixelRatio || 1)

  function accept(next: InvScanView) {
    if (view && next.seq < view.seq) return
    view = next
    dpr = window.devicePixelRatio || 1
  }

  onMount(() => {
    // Only this window is see-through and lets clicks through; the styles
    // are set here, not in global CSS, which every window's page shares.
    for (const el of [document.documentElement, document.body]) {
      el.style.setProperty('background', 'transparent', 'important')
      el.style.overflow = 'hidden'
      el.style.userSelect = 'none'
      el.style.pointerEvents = 'none'
    }
    AppService.GetInvScanView().then(accept).catch(() => {})
    const off = Events.On('invscan-view', (event) => accept(event.data as InvScanView))
    const offLang = followAppLanguage()
    return () => {
      off()
      offLang()
    }
  })

  // Text grows with the game's height (2160 = 1).
  const size = $derived(Math.max(12, (view?.scale || 1) * 22 / dpr))
  const messages: Record<string, string> = {
    loading: 'invscan.loading',
    closed: 'invscan.closed',
    button: 'invscan.button',
    error: 'invscan.error',
  }
</script>

{#if view}
  <div
    class="card"
    style:right="calc(100vw - {view.right / dpr}px)"
    style:top="{view.top / dpr}px"
    style:font-size="{size}px"
  >
    {#if view.message}
      <p class="message">{t(messages[view.message] ?? 'invscan.error')}</p>
    {:else}
      {#if view.map}
        <div class="map">
          <p>{t('invscan.mapRun', view.map.name)} <b>+{money(view.map.runEx, view.divineEx)}</b></p>
          {#if view.map.runs > 1}<p class="note">{t('invscan.mapTotal', view.map.runs, money(view.map.mapEx, view.divineEx))}</p>{/if}
          {#if view.map.noBase}<p class="note warn">{t('loot.noBase')}</p>{:else if !view.map.fresh}<p class="note warn">{t('loot.stale')}</p>{/if}
        </div>
      {:else if view.base}
        <p class="note">{t('invscan.based')}</p>
      {:else if view.inMap}
        <p class="note">{t('invscan.inMap')}</p>
      {/if}
      <h2>{t('invscan.title')}</h2>
      {#if view.rows?.length}
        <table>
          <tbody>
            {#each view.rows as row (row.name)}
              <tr class:unpriced={!row.eachEx}>
                <td class="count">{row.count}×</td>
                <td class="name">
                  <span
                    class="chip"
                    style:background={row.bg || undefined}
                    style:color={row.color || undefined}
                    style:border-color={row.border || undefined}
                  >{row.name}</span>
                </td>
                <td class="each">{row.eachEx ? money(row.eachEx, view.divineEx) : '?'}</td>
                <td class="total">{row.eachEx ? money(row.totalEx, view.divineEx) : ''}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="sum">{t('invscan.total')} <b>{money(view.totalEx, view.divineEx)}</b></p>
      {:else}
        <p class="message">{t('invscan.none')}</p>
      {/if}
      {#if view.unsure}
        <p class="note warn">{t('invscan.unsure', view.unsure)}</p>
        {#each view.doubts ?? [] as d (`${d.row},${d.col}`)}<p class="note warn doubt">{doubtText(d)}</p>{/each}
      {/if}
      {#if view.unpriced}<p class="note">{t('invscan.unpriced', view.unpriced)}</p>{/if}
      {#if view.off}<p class="note">{t('invscan.off', view.off)}</p>{/if}
    {/if}
  </div>
{/if}

<style>
  .card {
    position: fixed;
    max-height: 80vh;
    overflow: hidden;
    padding: 0.5em 0.8em 0.6em;
    border: 1px solid color-mix(in srgb, var(--ui-line-strong, #555042) 80%, transparent);
    border-radius: 4px;
    background: color-mix(in srgb, var(--ui-sunk, #111310) 92%, transparent);
    color: var(--ui-gold-bright, #e2d6b8);
    line-height: 1.35;
    white-space: nowrap;
  }
  h2 {
    margin: 0 0 0.35em;
    font-size: 0.85em;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--ui-muted, #9a9484);
    text-transform: uppercase;
  }
  table { border-collapse: collapse; }
  td { padding: 0.08em 0.35em; vertical-align: middle; }
  .count, .each, .total { text-align: right; font-variant-numeric: tabular-nums; }
  .count { color: var(--ui-muted, #9a9484); }
  .each { color: var(--ui-muted, #9a9484); font-size: 0.85em; }
  .total { font-weight: 600; }
  .chip {
    display: inline-block;
    padding: 0 0.4em;
    border: 1px solid transparent;
    border-radius: 2px;
  }
  .unpriced .chip { color: var(--ui-muted, #9a9484); }
  .sum {
    margin: 0.4em 0 0;
    padding-top: 0.35em;
    border-top: 1px solid color-mix(in srgb, var(--ui-line, #3e3a2f) 80%, transparent);
    text-align: right;
  }
  .sum b { font-size: 1.15em; }
  .note { margin: 0.25em 0 0; font-size: 0.8em; color: var(--ui-muted, #9a9484); }
  .warn { color: #e0a060; }
  .message { margin: 0; }
  .map {
    margin: 0 0 0.5em;
    padding-bottom: 0.4em;
    border-bottom: 1px solid color-mix(in srgb, var(--ui-line, #3e3a2f) 80%, transparent);
  }
  .map p { margin: 0; }
  .map b { font-size: 1.15em; color: var(--ui-gold-bright, #e2d6b8); }
</style>
