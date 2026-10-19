<script lang="ts">
  // The Expedition box: a small half-transparent box over the game with the
  // worth of the rewards chosen on the Runeshape panel, in the current map
  // and this game session (the league's are in the tracker window). It lets clicks
  // through and shows only while the game is in front; while being placed it
  // takes the mouse so it can be dragged.
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { ExpeditionEntry, ExpeditionHistoryView } from '../bindings/poe2filter/internal/app/models'
  import { boxDrag } from './lib/boxDrag'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { money } from './lib/format'

  let view = $state<ExpeditionHistoryView | null>(null)
  const placing = $derived(!!view?.placing)
  const shown = $derived((view?.entries?.length ?? 0) > 0)
  // The current map's choices, newest first (open entries lead the list).
  const NAME_ROWS = 8
  const open = $derived.by(() => {
    const out: ExpeditionEntry[] = []
    for (const e of view?.entries ?? []) {
      if (e.done) break
      out.push(e)
    }
    return out
  })

  function rewardName(e: ExpeditionEntry) {
    const name = e.display || e.name || e.text
    return e.countKnown && e.count > 1 ? `${e.count}× ${name}` : name
  }

  onMount(() => {
    for (const el of [document.documentElement, document.body]) {
      el.style.setProperty('background', 'transparent', 'important')
      el.style.overflow = 'hidden'
      el.style.userSelect = 'none'
    }
    AppService.GetExpeditionHistory().then((v) => (view = v)).catch(() => {})
    const off = Events.On('expedition-history', (event) => (view = event.data as ExpeditionHistoryView))
    const offLang = followAppLanguage()
    return () => {
      off()
      offLang()
    }
  })

  // Clicks go through to the game except while placing.
  $effect(() => {
    for (const el of [document.documentElement, document.body]) el.style.pointerEvents = placing ? 'auto' : 'none'
  })

  function worth(valueEx: number) {
    return valueEx > 0 ? money(valueEx, view?.divineEx ?? 0) : '0'
  }
</script>

{#if placing || shown}
  <div class="box" class:placing use:boxDrag={{ box: 'expedition' }}>
    {#if placing}
      <div class="line"><span class="k">⠿ {t('earnings.drag')}</span></div>
      <div class="line buttons">
        <button title={t('tracker.placeSave')} onclick={() => AppService.FinishExpeditionPlace(true)}>✓ {t('earnings.save')}</button>
        <button title={t('tracker.placeCancel')} onclick={() => AppService.FinishExpeditionPlace(false)}>✕</button>
      </div>
    {:else if view}
      <div class="line"><span class="k">{t('earnings.expedition')} · {t('earnings.map')}</span><span class="v num">{worth(view.mapTotal.valueEx)}</span></div>
      <div class="line"><span class="k">{t('earnings.expedition')} · {t('earnings.session')}</span><span class="v num">{worth(view.session.valueEx)}</span></div>
      {#if view.names && open.length > 0}
        <div class="names">
          {#each open.slice(0, open.length > NAME_ROWS + 1 ? NAME_ROWS : NAME_ROWS + 1) as e (e.at)}
            <div class="line name"><span class="n">{rewardName(e)}</span><span class="w num">{worth(e.valueEx)}</span></div>
          {/each}
          {#if open.length > NAME_ROWS + 1}
            <div class="line name more">{t('earnings.more', open.length - NAME_ROWS)}</div>
          {/if}
        </div>
      {/if}
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
    gap: 1px;
    padding: 3px 10px;
    border: 1px solid rgba(175, 145, 90, 0.5);
    border-radius: 5px;
    background: rgba(10, 8, 6, 0.5);
    color: #e8dcc0;
    font-size: 13px;
    line-height: 1.25;
    text-shadow: 0 1px 2px #000, 0 0 3px #000;
    overflow: hidden;
  }
  .box.placing {
    border: 1px dashed #f3d38a;
    background: rgba(10, 8, 6, 0.8);
    cursor: move;
  }
  .line {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    white-space: nowrap;
  }
  .k {
    color: #cbbd9c;
  }
  .v {
    color: #f3d38a;
    font-weight: 600;
  }
  .names {
    margin-top: 3px;
    padding-top: 3px;
    border-top: 1px solid rgba(175, 145, 90, 0.35);
  }
  .name {
    font-size: 12px;
    line-height: 17px;
  }
  .name .n {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .name .w {
    color: #e8dcc0;
  }
  .more {
    color: #cbbd9c;
  }
  .buttons {
    justify-content: flex-start;
    gap: 6px;
  }
  .buttons button {
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
