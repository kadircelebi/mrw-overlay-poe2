<script lang="ts">
  // The campaign box: over the game while the player is in a campaign area,
  // it says what to watch for there. It lets clicks through and is only as
  // tall as its notes; while being placed it takes the mouse and shows the
  // whole room it may need, so it can be dragged clear of the game's panels.
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { CampaignStatusView } from '../bindings/poe2filter/internal/app/models'
  import { boxDrag } from './lib/boxDrag'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { noteMark, say } from './lib/campaign'

  let view = $state<CampaignStatusView | null>(null)
  const placing = $derived(!!view?.placing)
  const area = $derived(view?.area ?? null)

  onMount(() => {
    for (const el of [document.documentElement, document.body]) {
      el.style.setProperty('background', 'transparent', 'important')
      el.style.overflow = 'hidden'
      el.style.userSelect = 'none'
    }
    AppService.GetCampaignStatus().then((v) => (view = v)).catch(() => {})
    const off = Events.On('campaign-status', (event) => (view = event.data as CampaignStatusView))
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
</script>

{#if placing}
  <div class="box placing" use:boxDrag={{ box: 'campaign' }}>
    <div class="line"><span class="drag">⠿ {t('earnings.drag')}</span></div>
    <div class="buttons">
      <button title={t('tracker.placeSave')} onclick={() => AppService.FinishCampaignBoxPlace(true)}>✓ {t('earnings.save')}</button>
      <button title={t('tracker.placeCancel')} onclick={() => AppService.FinishCampaignBoxPlace(false)}>✕</button>
    </div>
  </div>
{:else if area}
  <div class="box">
    <div class="head">
      <span class="name" lang="en">{area.name}</span>
      {#if view}<span class="act">{say({ en: view.actEn, tr: view.actTr })}</span>{/if}
    </div>
    <ul>
      {#each area.notes as note, i (i)}
        <li class={note.kind}><span class="mark">{noteMark(note.kind)}</span><span>{say(note)}</span></li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .box {
    box-sizing: border-box;
    width: 100vw;
    max-height: 100vh;
    padding: 5px 10px 6px;
    border: 1px solid rgba(175, 145, 90, 0.5);
    border-radius: 5px;
    background: rgba(10, 8, 6, 0.62);
    color: #e8dcc0;
    font-size: 13px;
    line-height: 17px;
    text-shadow: 0 1px 2px #000, 0 0 3px #000;
    overflow: hidden;
  }
  .box.placing {
    height: 100vh;
    border: 1px dashed #f3d38a;
    background: rgba(10, 8, 6, 0.8);
    cursor: move;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 3px;
    padding-bottom: 3px;
    border-bottom: 1px solid rgba(175, 145, 90, 0.35);
  }
  .name {
    color: #f3d38a;
    font-weight: 600;
  }
  .act {
    flex: none;
    max-width: 55%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: #a89a7c;
    font-size: 11.5px;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    gap: 6px;
  }
  .mark {
    flex: none;
    width: 12px;
    text-align: center;
  }
  li.must {
    color: #f3d38a;
    font-weight: 600;
  }
  li.optional {
    color: #b9c9a8;
  }
  li.tip {
    color: #d8ccb0;
  }
  li.next {
    color: #a9cbe8;
  }
  .drag {
    color: #cbbd9c;
  }
  .buttons {
    display: flex;
    gap: 6px;
    margin-top: 2px;
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
