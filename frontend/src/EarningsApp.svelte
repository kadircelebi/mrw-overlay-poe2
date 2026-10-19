<script lang="ts">
  // The earnings overlay: a small half-transparent box over the game with
  // the merchant sales of this game session and of the league, in Divine
  // Orbs. It lets clicks through and shows only while the game is in front;
  // while being placed it takes the mouse so it can be dragged.
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { TradeHistoryView } from '../bindings/poe2filter/internal/app/models'
  import { boxDrag } from './lib/boxDrag'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'

  let view = $state<TradeHistoryView | null>(null)
  const placing = $derived(!!view?.placing)

  onMount(() => {
    for (const el of [document.documentElement, document.body]) {
      el.style.setProperty('background', 'transparent', 'important')
      el.style.overflow = 'hidden'
      el.style.userSelect = 'none'
    }
    AppService.GetTradeHistory().then((v) => (view = v)).catch(() => {})
    const off = Events.On('trade-history', (event) => (view = event.data as TradeHistoryView))
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

  // Exalted Orbs as Divine Orbs at today's rate.
  function div(valueEx: number) {
    if (!view || !(view.divineEx > 0)) return '—'
    const d = valueEx / view.divineEx
    return `${d >= 100 ? d.toFixed(0) : d >= 10 ? d.toFixed(1) : d.toFixed(2)} div`
  }
</script>

{#if view?.enabled}
  <div class="box" class:placing use:boxDrag={{ box: 'earnings' }}>
    {#if placing}
      <div class="line"><span class="k">⠿ {t('earnings.drag')}</span></div>
      <div class="line buttons">
        <button title={t('tracker.placeSave')} onclick={() => AppService.FinishEarningsPlace(true)}>✓ {t('earnings.save')}</button>
        <button title={t('tracker.placeCancel')} onclick={() => AppService.FinishEarningsPlace(false)}>✕</button>
      </div>
    {:else}
      <div class="line"><span class="k">{t('earnings.session')}</span><span class="v num">{div(view.session.valueEx)}</span></div>
      <div class="line"><span class="k">{t('earnings.league')}</span><span class="v num">{div(view.leagueTotal.valueEx)}</span></div>
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
