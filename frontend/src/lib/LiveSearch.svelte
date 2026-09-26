<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../../bindings/poe2filter'
  import type { SearchLibrary } from '../../bindings/poe2filter/internal/overlay/models'
  import type { EvaluatedListing, Evaluation, LiveState } from '../../bindings/poe2filter/internal/trade/models'
  import TradeResults from './TradeResults.svelte'
  import { t } from './i18n.svelte'
  import { listedAgo } from './overlayQuery'

  // The market's Live Search tab: every saved search can be started here. GGG
  // then pushes new listings; the app notifies and plays a sound (Go side),
  // and this list shows what each search found.
  let { library }: { library: SearchLibrary } = $props()

  const MAX_LIVE = 20
  let states = $state<Record<string, LiveState>>({})
  let selected = $state('')
  let results = $state<EvaluatedListing[]>([])
  let error = $state('')
  let busy = $state('')
  let signedIn = $state(true)
  let sound = $state('ShExalted')
  let notify = $state(true)
  let soundIds = $state<string[]>([])

  const searches = $derived(library.searches ?? [])
  const folderName = $derived(Object.fromEntries((library.folders ?? []).map((folder) => [folder.id, folder.name])))
  const active = $derived(Object.values(states).filter((state) => isRunning(state.status)).length)
  const current = $derived(states[selected])
  const result = $derived<Evaluation | null>(selected ? {
    searchId: current?.searchId ?? '', tradeUrl: current?.tradeUrl ?? '', total: results.length,
    resultIds: [], listings: results, signedIn,
  } : null)

  function isRunning(status: string) { return status === 'connecting' || status === 'live' || status === 'reconnecting' }

  function adopt(list: LiveState[] | null) {
    states = Object.fromEntries((list ?? []).map((state) => [state.id, state]))
  }

  async function loadResults() {
    if (!selected) { results = []; return }
    const id = selected
    const list = await AppService.LiveResults(id)
    if (id === selected) results = list ?? []
  }

  onMount(() => {
    AppService.LiveSearches().then(adopt)
    AppService.GetOverlaySettings().then((settings) => { sound = settings.live_sound || 'ShExalted'; notify = settings.live_notify })
    AppService.StyleOptions().then((options) => (soundIds = options.sounds ?? []))
    AppService.BrowserLinkState().then((link) => (signedIn = link.connected)).catch(() => {})
    const offState = Events.On('live-state', (event) => adopt(event.data as LiveState[]))
    const offFound = Events.On('live-found', (event) => {
      if ((event.data as LiveState).id === selected) loadResults()
    })
    return () => { offState(); offFound() }
  })

  function select(id: string) {
    selected = id
    loadResults()
  }

  async function start(id: string) {
    busy = id
    error = ''
    try {
      adopt(await AppService.StartLiveSearch(id))
      if (!selected) select(id)
    } catch (e) {
      error = String(e).replace(/^RuntimeError:\s*/i, '')
    } finally {
      busy = ''
    }
  }

  async function stop(id: string) {
    adopt(await AppService.StopLiveSearch(id))
  }

  async function clear() {
    if (!selected) return
    adopt(await AppService.ClearLiveResults(selected))
    results = []
  }

  async function saveAlerts() {
    try {
      const settings = await AppService.SetLiveAlerts(sound, notify)
      sound = settings.live_sound
      notify = settings.live_notify
    } catch (e) {
      error = String(e).replace(/^RuntimeError:\s*/i, '')
    }
  }

  const soundCurrency: Record<string, string> = {
    ShAlchemy: 'Orb of Alchemy', ShBlessed: 'Blessed Orb', ShChaos: 'Chaos Orb', ShFusing: 'Orb of Fusing',
    ShGeneral: 'Orb of immense power', ShRegal: 'Regal Orb', ShVaal: 'Vaal Orb', ShDivine: 'Divine Orb',
    ShExalted: 'Exalted Orb', ShMirror: 'Mirror of Kalandra',
  }
  function soundLabel(id: string, i: number) { return soundCurrency[id] ? `${i + 1} · ${soundCurrency[id]}` : id }

  function statusLabel(state?: LiveState) {
    if (!state) return t('live.status.idle')
    return t(`live.status.${state.status}`)
  }
</script>

<div class="live-root">
  <section class="live-list">
    {#if !signedIn}
      <div class="notice">
        <p>{t('live.needLogin')}</p>
        <button type="button" onclick={() => AppService.ShowSettings('account')}>{t('live.openAccount')}</button>
      </div>
    {/if}
    <div class="alerts">
      <label><span>{t('live.sound')}</span>
        <select bind:value={sound} onchange={saveAlerts}>
          <option value="none">{t('live.soundNone')}</option>
          {#each soundIds as id, i}<option value={id}>{soundLabel(id, i)}</option>{/each}
        </select>
      </label>
      <button type="button" class="play" title={t('live.preview')} disabled={sound === 'none'} onclick={() => AppService.PreviewGameSound(sound)}>▶</button>
      <label class="check"><input type="checkbox" bind:checked={notify} onchange={saveAlerts} /><i></i><span>{t('live.notify')}</span></label>
    </div>
    <div class="table-head"><span>{t('live.col.name')}</span><span>{t('live.col.status')}</span><span>{t('live.col.found')}</span><span class:full={active >= MAX_LIVE}>{t('live.active', active, MAX_LIVE)}</span></div>
    {#if error}<p class="error">{error}</p>{/if}
    {#each searches as saved (saved.id)}
      {@const state = states[saved.id]}
      {@const running = !!state && isRunning(state.status)}
      <div class="row" class:on={selected === saved.id} class:running>
        <button type="button" class="name" onclick={() => select(saved.id)}>
          <i class="dot {state?.status ?? 'idle'}"></i>
          <span><b>{saved.name}</b>{#if saved.folder && folderName[saved.folder]}<small>{folderName[saved.folder]}</small>{/if}</span>
        </button>
        <span class="status {state?.status ?? 'idle'}" title={state?.error ?? ''}>{statusLabel(state)}</span>
        <span class="found" title={state?.lastFoundMs ? listedAgo(new Date(state.lastFoundMs).toISOString()) : ''}>{state?.found ? state.found : '—'}</span>
        {#if running}
          <button type="button" class="toggle stop" onclick={() => stop(saved.id)}>{t('live.stop')} ■</button>
        {:else}
          <button type="button" class="toggle" disabled={busy === saved.id || active >= MAX_LIVE} onclick={() => start(saved.id)}>{t('live.start')} ▶</button>
        {/if}
      </div>
      {#if state?.error}<p class="row-error">{state.error}</p>{/if}
    {:else}
      <p class="empty">{t('live.noSaved')}</p>
    {/each}
  </section>
  <section class="live-results">
    {#if selected}
      {@const saved = searches.find((search) => search.id === selected)}
      <div class="results-title">
        <strong>{saved?.name ?? ''}</strong>
        <button type="button" disabled={!results.length} onclick={clear}>{t('live.clear')}</button>
      </div>
      {#if results.length}
        <TradeResults {result} expanded />
      {:else}
        <p class="empty">{current && isRunning(current.status) ? t('live.waiting') : t('live.notStarted')}</p>
      {/if}
    {:else}
      <p class="empty">{t('live.pick')}</p>
    {/if}
  </section>
</div>

<style>
  .live-root { min-height: 0; flex: 1; display: grid; grid-template-columns: minmax(330px, 1fr) minmax(280px, 1fr); overflow: hidden; }
  .live-list { min-width: 0; overflow: auto; padding: 8px; border-right: 1px solid #37372f; }
  .live-results { min-width: 0; overflow: auto; padding: 8px; background: #141612; }
  .notice { margin-bottom: 8px; padding: 8px; border: 1px solid #6d5a36; background: #1d1a12; color: #d9c79a; font-size: 10px; }
  .notice p { margin: 0 0 6px; }
  .notice button { padding: 5px 8px; border: 1px solid #8c7b50; background: #20221e; color: var(--gold-bright); font-size: 9px; }
  .alerts { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; padding: 6px; border: 1px solid #30322b; background: #11130f; font-size: 9px; color: #b8b09a; }
  .alerts label { display: flex; align-items: center; gap: 5px; }
  .alerts select { min-width: 0; max-width: 150px; padding: 4px; border: 1px solid #414139; background: #20221d; color: #ccc6b2; font-size: 9px; }
  .alerts .play { width: 22px; height: 22px; padding: 0; border: 1px solid #56523f; background: #181a17; color: #a99c72; font-size: 9px; }
  .alerts .play:disabled { opacity: .35; }
  .alerts .check { margin-left: auto; cursor: pointer; }
  .alerts .check input { position: absolute; opacity: 0; }
  .alerts .check i { width: 10px; height: 10px; transform: rotate(45deg); border: 1px solid var(--gold-dim); }
  .alerts .check input:checked + i { background: var(--gold); box-shadow: inset 0 0 0 3px #17191b; }
  .table-head, .row { display: grid; grid-template-columns: minmax(0, 1fr) 66px 36px 72px; align-items: center; gap: 5px; }
  .table-head { padding: 6px 8px; border: 1px solid #30322b; border-bottom: 0; background: #20221d; color: #d6ccb0; font-size: 9px; }
  .table-head span:nth-child(n+2) { text-align: center; }
  .table-head .full { color: #df8179; }
  .row { padding: 6px 8px; border: 1px solid #2b2d27; border-top-width: 0; background: #151714; }
  .row.on { background: #23251e; box-shadow: inset 2px 0 0 var(--gold); }
  .row .name { display: flex; align-items: center; gap: 7px; min-width: 0; padding: 0; border: 0; background: none; text-align: left; }
  .row .name span { min-width: 0; display: flex; flex-direction: column; }
  .row .name b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #d2c8aa; font-size: 10px; font-weight: 600; }
  .row .name small { color: #737a78; font-size: 8.5px; }
  .dot { flex: 0 0 9px; width: 9px; height: 9px; border: 1px solid #5d4a63; background: #4a2250; }
  .dot.live { background: #5fbf5a; border-color: #8fe08a; box-shadow: 0 0 6px #5fbf5a; animation: pulse 1.6s infinite alternate; }
  .dot.connecting, .dot.reconnecting { background: #c9a44a; border-color: #e7c979; }
  .dot.error { background: #b8483f; border-color: #e0776d; }
  .status { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; color: #8d918a; font-size: 9px; }
  .status.live { color: #8fd48a; }
  .status.connecting, .status.reconnecting { color: #d8bd73; }
  .status.error { color: #df8179; }
  .found { text-align: center; color: #e7d8a8; font-size: 10px; font-weight: bold; }
  .toggle { white-space: nowrap; padding: 6px 4px; border: 1px solid #8c7b50; background: #191b17; color: var(--gold-bright); font-size: 9.5px; font-weight: bold; }
  .toggle:hover:not(:disabled) { background: #25261f; }
  .toggle:disabled { opacity: .4; }
  .toggle.stop { border-color: #7a3a34; color: #f0b5ad; }
  .row-error { margin: 0; padding: 4px 8px 6px 24px; border: 1px solid #2b2d27; border-top: 0; background: #151714; color: #df8179; font-size: 9px; }
  .error { margin: 6px 0; color: #df8179; font-size: 9.5px; }
  .empty { padding: 16px; color: var(--muted); text-align: center; font-size: 10px; }
  .results-title { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; color: #d6ccb0; font-family: var(--serif); font-size: 11px; }
  .results-title button { padding: 4px 8px; border: 1px solid #45443a; background: none; color: #a99c72; font-family: inherit; font-size: 9px; }
  .results-title button:disabled { opacity: .35; }
  @keyframes pulse { to { opacity: .55; } }
</style>
