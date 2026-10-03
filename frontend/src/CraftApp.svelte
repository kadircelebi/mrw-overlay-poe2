<script lang="ts">
  import { onMount } from 'svelte'
  import { Events, Window } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter'
  import { allOn, buildRequest, choicesFor, modifiableFilters, searchedStats } from './lib/overlayQuery'
  import { t, currentLang } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import TradeResults from './lib/TradeResults.svelte'
  import type { Item } from '../bindings/poe2filter/internal/overlay/models'
  import type { EvaluateRequest, Evaluation } from '../bindings/poe2filter/internal/trade/models'

  let frame: HTMLIFrameElement
  // The craft window fills the screen on demand (button, double click on the
  // title bar, F11 in either page) and remembers it while hidden.
  let maximised = $state(false)
  async function toggleSize() {
    try { await Window.ToggleMaximise(); maximised = await Window.IsMaximised() } catch { /* not in the app */ }
  }
  let error = $state('')
  let searching = false
  // The price panel: the crafted item searched on the trade site, right
  // beside the craft. Broad (-10%) is the default; a crafted item with every
  // roll taken exactly rarely has a listing.
  let price = $state<{ raw: string; item: Item; broad: boolean; query: EvaluateRequest | null;
    result: Evaluation | null; loading: boolean; error: string; searched: string[] } | null>(null)
  const cleanError = (value: unknown) => String(value).replace(/^RuntimeError:\s*/i, '')
  function priceQuery(item: Item, broad: boolean): EvaluateRequest {
    return buildRequest(item, choicesFor(item, broad), 'securable', [...modifiableFilters], [], { ...allOn, base: !!item.baseType })
  }
  async function runPrice(broad: boolean) {
    if (!price || price.loading) return
    price.broad = broad
    price.loading = true
    price.error = ''
    try {
      const query = priceQuery(price.item, broad)
      price.query = query
      price.searched = searchedStats(query)
      price.result = await AppService.EvaluateOverlay(query, false)
    } catch (e) {
      price.error = cleanError(e)
      price.result = null
    } finally {
      price.loading = false
    }
  }
  async function openMarket() {
    if (!price?.query) return
    try { await AppService.ShowCraftMarketWithQuery(price.raw, price.query) }
    catch (e) { price.error = cleanError(e) }
  }
  let iconsSent = false
  // The craft page has its own texts; it opens in the app's language and
  // follows later changes.
  const frameSrc = `/craft/index.html?lang=${encodeURIComponent(currentLang())}`
  const sendLang = () => send({ type: 'craft-lang', lang: currentLang() })
  // An item sent from the price check waits until the page has loaded.
  let frameReady = false
  let pendingImport: unknown = null
  function sendImport() {
    if (frameReady && pendingImport) { send({ type: 'craft-import', item: pendingImport }); pendingImport = null }
  }
  $effect(sendLang)

  function send(data: unknown) { frame?.contentWindow?.postMessage(data, location.origin) }
  async function refreshPrices() {
    try { send({ type: 'craft-prices', prices: await AppService.GetCraftPrices() }) }
    catch (e) { error = String(e) }
  }
  // The craft page draws currency icons from the trade site's list (the art
  // is not shipped in the package). Without it the buttons show a fallback.
  async function sendIcons() {
    try { send({ type: 'craft-icons', currencies: (await AppService.TradeCurrencies()) ?? [] }); iconsSent = true }
    catch { /* offline: fallbacks stay */ }
  }
  onMount(() => {
    const offLang = followAppLanguage()
    const offPrices = Events.On('state', () => void refreshPrices())
    const offImport = Events.On('craft-import', (ev) => { pendingImport = ev.data; sendImport() })
    const receive = async (event: MessageEvent) => {
      if (event.source !== frame?.contentWindow || event.origin !== location.origin) return
      if (event.data?.type === 'craft-size') { void toggleSize(); return }
      if (event.data?.type === 'craft-ready') { frameReady = true; sendLang(); sendImport(); void sendIcons(); await refreshPrices(); return }
      if (event.data?.type !== 'craft-price' || typeof event.data.raw !== 'string' || searching) return
      searching = true; error = ''
      try {
        const snap = await AppService.ParseCraftText(event.data.raw)
        if (!snap.item) throw new Error('Craft item could not be parsed')
        const item = snap.item
        const unmatched = (item.mods ?? []).filter(m => m.type !== 'pseudo' && !m.statId)
        if (unmatched.length) throw new Error(t('craft.unmatched') + unmatched.map(m => m.text).join('; '))
        price = { raw: event.data.raw, item, broad: price?.broad ?? true, query: null, result: null, loading: false, error: '', searched: [] }
        await runPrice(price.broad)
        send({ type: 'craft-result', error: price?.error || '', message: t('craft.priced') })
      } catch (e) {
        error = String(e); send({ type:'craft-result', error })
      } finally { searching = false }
    }
    window.addEventListener('message', receive)
    const onFocus = () => { void refreshPrices(); if (!iconsSent) void sendIcons() }
    window.addEventListener('focus', onFocus)
    const onKey = (e: KeyboardEvent) => { if (e.key === 'F11') { e.preventDefault(); void toggleSize() } }
    window.addEventListener('keydown', onKey)
    Window.IsMaximised().then((v) => (maximised = v)).catch(() => {})
    return () => {
      offLang(); offPrices(); offImport()
      window.removeEventListener('message', receive)
      window.removeEventListener('focus', onFocus)
      window.removeEventListener('keydown', onKey)
    }
  })
</script>

<main class="craft-shell">
  <header style="--wails-draggable:drag" role="toolbar" tabindex="-1" ondblclick={(e) => { if (!(e.target as HTMLElement).closest('button')) void toggleSize() }}>
    <img src="/emblem.png" alt="" /><strong>MrW Overlay · {t('craft.open')}</strong>
    <button class="size" title={maximised ? t('craft.restoreSize') : t('craft.fullscreen')} aria-label={maximised ? t('craft.restoreSize') : t('craft.fullscreen')}
      aria-pressed={maximised} onclick={toggleSize}>{maximised ? '❐' : '□'}</button>
    <button title={t('window.close')} aria-label={t('window.close')} onclick={() => AppService.HideCraft()}>×</button>
  </header>
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  <div class="body">
    <iframe bind:this={frame} src={frameSrc} title={t('craft.open')}></iframe>
    {#if price}
      <aside class="price" aria-label={t('craft.priceTitle')}>
        <div class="price-head">
          <strong>{t('craft.priceTitle')}</strong>
          <button class="close" title={t('window.close')} aria-label={t('window.close')} onclick={() => (price = null)}>×</button>
        </div>
        <p class="price-item">{price.item.baseType || price.item.class} · {t('craft.priceMods', price.searched.length)}</p>
        <div class="price-tools">
          <div class="mode" role="group">
            <button class:on={!price.broad} disabled={price.loading} onclick={() => runPrice(false)}>{t('ov.exact')}</button>
            <button class:on={price.broad} disabled={price.loading} onclick={() => runPrice(true)}>{t('ov.broad')}</button>
          </div>
          <button class="market" disabled={!price.query} onclick={openMarket}>{t('craft.openMarket')}</button>
        </div>
        {#if price.result && !price.loading && !price.error && (price.result.total ?? 0) === 0}
          <p class="hint">{price.broad ? t('craft.priceNoneBroad') : t('craft.priceNone')}</p>
        {/if}
        <div class="results">
          <TradeResults result={price.result} loading={price.loading} error={price.error} searched={price.searched} expanded />
        </div>
      </aside>
    {/if}
  </div>
</main>

<style>
  .craft-shell { height:100%; display:flex; flex-direction:column; border:1px solid var(--line-strong); background:var(--grain),var(--bg); }
  header { display:flex; align-items:center; gap:8px; padding:7px 10px; border-bottom:1px solid var(--line-strong); flex-shrink:0; }
  header img { width:24px; height:24px; }
  header strong { color:var(--gold-bright); font:500 14px var(--serif); flex:1; }
  header button { --wails-draggable:no-drag; padding:1px 8px; font-size:22px; }
  header button.size { font-size:17px; padding:3px 9px; }
  .body { flex:1; min-height:0; display:flex; position:relative; }
  iframe { width:100%; flex:1; min-height:0; border:0; background:var(--bg); }
  .price { position:absolute; top:0; right:0; bottom:0; width:min(470px, 92%); display:flex; flex-direction:column; gap:8px;
    padding:10px 12px; background:var(--surface, var(--bg)); border-left:1px solid var(--line-strong); box-shadow:-12px 0 28px rgba(0,0,0,.45); }
  .price-head { display:flex; align-items:center; gap:8px; }
  .price-head strong { flex:1; color:var(--gold-bright); font:500 14px var(--serif); }
  .price-head .close { padding:1px 8px; font-size:20px; }
  .price-item { margin:0; color:var(--muted); font-size:12px; }
  .price-tools { display:flex; gap:8px; align-items:center; justify-content:space-between; flex-wrap:wrap; }
  .mode { display:flex; }
  .mode button { font-size:12px; padding:4px 10px; }
  .mode button.on { border-color:var(--gold-dim); color:var(--gold-bright); }
  .market { font-size:12px; padding:4px 10px; }
  .hint { margin:0; color:var(--muted); font-size:12px; }
  .results { flex:1; min-height:0; overflow-y:auto; }
  .error { color:var(--bad); padding:8px; font-size:12px; }
</style>
