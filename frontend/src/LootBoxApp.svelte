<script lang="ts">
  // The loot box: a small half-transparent box over the game, in maps too,
  // with the lines the player picked in the tracker window: whether the
  // next map's base is read (and when), the live clock, the inventory or
  // the read the map began from, the last map and the session's figures. It
  // lets clicks through and shows only while the game is in front; while
  // being placed it takes the mouse so it can be dragged.
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { LootStatusView } from '../bindings/poe2filter/internal/app/models'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { clock, money } from './lib/format'
  import { fieldLabel, fieldValue, timerText, type FarmData } from './lib/farm'

  // How long the last map's line stands out once it comes in.
  const FLASH_MS = 8000

  let view = $state<LootStatusView | null>(null)
  let now = $state(Date.now())
  const placing = $derived(!!view?.placing)

  onMount(() => {
    for (const el of [document.documentElement, document.body]) {
      el.style.setProperty('background', 'transparent', 'important')
      el.style.overflow = 'hidden'
      el.style.userSelect = 'none'
    }
    AppService.GetLootStatus().then((v) => (view = v)).catch(() => {})
    const off = Events.On('loot-status', (event) => (view = event.data as LootStatusView))
    const offLang = followAppLanguage()
    const tick = setInterval(() => (now = Date.now()), 500)
    return () => {
      off()
      offLang()
      clearInterval(tick)
    }
  })

  // Clicks go through to the game except while placing.
  $effect(() => {
    for (const el of [document.documentElement, document.body]) el.style.pointerEvents = placing ? 'auto' : 'none'
  })

  function worth(valueEx: number) {
    return money(valueEx, view?.divineEx ?? 0)
  }

  const manual = $derived(view?.mode === 'manual')
  const hotkey = $derived(view?.hotkey || 'Alt+I')
  const hasBase = $derived((view?.baseAt ?? 0) > 0)
  // The state line: its words and its light (ok, wait, bad, idle).
  const status = $derived.by(() => {
    const v = view
    if (!v) return { text: '', tone: 'idle' }
    const read = manual ? t('lootbox.press', hotkey) : t('lootbox.open')
    switch (v.phase) {
      case 'noBase':
        return { text: `${t('lootbox.noBase')} · ${read}`, tone: 'bad' }
      case 'based':
        return v.fresh
          ? { text: t('lootbox.based', clock(v.baseAt)), tone: 'ok' }
          : { text: `${t('lootbox.stale')} · ${read}`, tone: 'wait' }
      case 'inMap':
        if (!v.map) return { text: t('lootbox.inMap', '—'), tone: 'idle' }
        if (!hasBase) return { text: `${t('lootbox.inMap', v.map)} · ${t('lootbox.noBaseShort')}`, tone: 'bad' }
        return { text: t('lootbox.inMap', v.map), tone: v.fresh ? 'ok' : 'wait' }
      case 'back':
        return { text: `${t('lootbox.back', v.map)} · ${read}`, tone: 'wait' }
      case 'reading':
        return { text: t('lootbox.reading', Math.max(0, Math.ceil((v.readingUntil - now) / 1000))), tone: 'wait' }
    }
    return { text: t('lootbox.unknown'), tone: 'idle' }
  })
  // In a hideout the inventory as last read; elsewhere the read the map
  // began from.
  const baseLabel = $derived(
    view?.phase === 'noBase' || view?.phase === 'based' || view?.phase === 'unknown'
      ? t('lootbox.inventory')
      : hasBase
        ? t('lootbox.base', clock(view!.baseAt))
        : t('lootbox.baseNone'),
  )
  const last = $derived(view?.lastRun ?? null)
  const lastWarn = $derived(!!last && (last.noBase || !last.fresh || last.unsure > 0))
  const flash = $derived(!!view && view.lastRunAt > 0 && now - view.lastRunAt < FLASH_MS)
  const data = $derived<FarmData>({ farm: view!.farm, session: view!.session, divineEx: view?.divineEx ?? 0, lastRun: last })
</script>

{#if view}
  <div class="box" class:placing>
    {#if placing}
      <div class="line"><span class="k">⠿ {t('earnings.drag')}</span></div>
      <div class="line buttons">
        <button title={t('tracker.placeSave')} onclick={() => AppService.FinishLootBoxPlace(true)}>✓ {t('earnings.save')}</button>
        <button title={t('tracker.placeCancel')} onclick={() => AppService.FinishLootBoxPlace(false)}>✕</button>
      </div>
    {:else}
      {#each view.fields ?? [] as key (key)}
        {#if key === 'status'}
          <div class="line state {status.tone}"><span class="dot"></span><span class="s">{status.text}</span></div>
        {:else if key === 'timer'}
          <div class="line"><span class="k">{timerText(view.farm, now)}</span></div>
        {:else if key === 'inventory'}
          <div class="line">
            <span class="k">{baseLabel}{#if hasBase && view.unsure}&nbsp;<span class="warn" title={t('loot.unsure', view.unsure)}>?</span>{/if}</span>
            <span class="v num">{hasBase ? worth(view.invEx) : '—'}</span>
          </div>
        {:else if key === 'lastGain'}
          <div class="line" class:flash>
            <span class="k n">{fieldLabel(key, data)}</span>
            <span class="v num">{#if lastWarn}<span class="warn">⚠</span>&nbsp;{/if}{fieldValue(key, data, now)}</span>
          </div>
        {:else}
          <div class="line" class:flash={key === 'lastTime' && flash}>
            <span class="k n">{fieldLabel(key, data)}</span>
            <span class="v num">{fieldValue(key, data, now)}</span>
          </div>
        {/if}
      {/each}
    {/if}
  </div>
{/if}

<style>
  .box {
    box-sizing: border-box;
    height: 100vh;
    display: flex;
    flex-direction: column;
    justify-content: center;
    padding: 3px 10px;
    border: 1px solid rgba(175, 145, 90, 0.5);
    border-radius: 5px;
    background: rgba(10, 8, 6, 0.5);
    color: #e8dcc0;
    font-size: 13px;
    line-height: 17px;
    text-shadow: 0 1px 2px #000, 0 0 3px #000;
    overflow: hidden;
  }
  .box.placing {
    border: 1px dashed #f3d38a;
    background: rgba(10, 8, 6, 0.8);
    cursor: move;
    --wails-draggable: drag;
  }
  .line {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    white-space: nowrap;
    border-radius: 3px;
    transition: background 0.4s;
  }
  .k {
    color: #cbbd9c;
  }
  .k.n {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .v {
    color: #f3d38a;
    font-weight: 600;
  }
  .warn {
    color: #e8a54a;
  }
  .flash {
    background: rgba(243, 211, 138, 0.22);
  }
  .state {
    justify-content: flex-start;
    gap: 6px;
  }
  .state .s {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #8a8578;
    box-shadow: 0 0 3px #000;
  }
  .ok .dot { background: #6fcf6a; }
  .wait .dot { background: #e8b04a; }
  .bad .dot { background: #e0563f; }
  .wait .s { color: #f0d49a; }
  .bad .s { color: #f0a090; }
  .buttons {
    justify-content: flex-start;
    gap: 6px;
  }
  .buttons button {
    --wails-draggable: no-drag;
    border: 1px solid rgba(175, 145, 90, 0.7);
    border-radius: 3px;
    background: #2a2217;
    color: #f3d38a;
    font-size: 11.5px;
    padding: 0 7px;
    line-height: 17px;
    cursor: pointer;
  }
</style>
