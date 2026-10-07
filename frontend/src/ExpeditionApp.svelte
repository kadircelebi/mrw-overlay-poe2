<script lang="ts">
  // Price labels laid over the game beside Expedition's Runeshape
  // Combinations panel. The window covers the game's client area and lets
  // clicks through; positions come in the game's physical pixels.
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { ExpeditionPrice, ExpeditionView } from '../bindings/poe2filter/internal/app/models'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { money } from './lib/format'

  let view = $state<ExpeditionView | null>(null)
  let dpr = $state(window.devicePixelRatio || 1)

  function accept(next: ExpeditionView) {
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
    AppService.GetExpeditionView().then(accept).catch(() => {})
    const off = Events.On('expedition-view', (event) => accept(event.data as ExpeditionView))
    const offLang = followAppLanguage()
    return () => {
      off()
      offLang()
    }
  })

  // Each label also names what was read (the matched item, or the raw text
  // when none matched), so a misread shows and players can report it.
  const SHOW_READ_NAME = true

  // A reward's worth; with its count unread, the price of one.
  function price(row: ExpeditionPrice) {
    let text = row.pending ? '…' : '?'
    if (row.valueEx && view) {
      const amount = money(row.valueEx, view.divineEx)
      text = row.countKnown ? amount : t('expedition.each', amount)
    }
    if (SHOW_READ_NAME) text += ` · ${row.name && row.countKnown && row.count > 1 ? `${row.count}× ` : ''}${row.name || row.text}`
    return text
  }
</script>

{#if view}
  {#each view.rows ?? [] as row}
    <div
      class="label"
      class:below={row.below}
      class:unknown={!row.valueEx}
      style:left="{(view.panelRight + row.h * 0.6) / dpr}px"
      style:top="{(row.y + row.h / 2) / dpr}px"
      style:font-size="{Math.max(12, (row.h / dpr) * 0.62)}px"
      style:background={row.bg || undefined}
      style:color={row.color || undefined}
      style:border-color={row.border || undefined}
    >{price(row)}</div>
  {/each}
  {#if view.message}
    <div class="label message">{t(view.message === 'notFound' ? 'expedition.notFound' : 'expedition.error')}</div>
  {/if}
{/if}

<style>
  .label {
    position: fixed;
    transform: translateY(-50%);
    padding: 0.15em 0.6em;
    border: 1px solid color-mix(in srgb, var(--ui-line-strong, #555042) 80%, transparent);
    border-radius: 3px;
    background: color-mix(in srgb, var(--ui-sunk, #111310) 88%, transparent);
    color: var(--ui-gold-bright, #e2d6b8);
    font-weight: 600;
    line-height: 1.35;
    white-space: nowrap;
  }
  .below { opacity: 0.5; }
  .unknown { color: var(--ui-muted, #9a9484); }
  .message {
    left: 2vw;
    top: 12vh;
    transform: none;
    font-size: 15px;
  }
</style>
