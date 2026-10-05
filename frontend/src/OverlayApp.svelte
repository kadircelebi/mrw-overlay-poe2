<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { Catalog, CurrencyQuote, Item, ItemEntry, Snapshot } from '../bindings/poe2filter/internal/overlay/models'
  import type { Evaluation, SelectedFilter } from '../bindings/poe2filter/internal/trade/models'
  import CurrencyCard from './lib/CurrencyCard.svelte'
  import FilterVerdict from './lib/FilterVerdict.svelte'
  import HidePanel from './lib/HidePanel.svelte'
  import ExoticAdd from './lib/ExoticAdd.svelte'
  import type { ExoticCandidate, FilterExplanation } from '../bindings/poe2filter/internal/app/models'
  import QuotaBadge from './lib/QuotaBadge.svelte'
  import OverlayItemCard from './lib/OverlayItemCard.svelte'
  import TradeResults from './lib/TradeResults.svelte'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { searchedStats, allOn, modifiableFilters, buildRequest, categoryFor, choicesFor, isGem, propertyFiltersFor, resetChoiceRanges, resetPropertyRanges, type ItemToggles, type ModChoice, type PropertyFilter } from './lib/overlayQuery'

  let item = $state<Item | null>(null)
  // Set for stackable items the price list knows: they get a worth card
  // instead of the affix card.
  let quote = $state<CurrencyQuote | null>(null)
  // Set once the price list has answered for the current item.
  let quoteChecked = $state(false)
  let catalog = $state<Catalog | null>(null)
  let choices = $state<ModChoice[]>([])
  let toggles = $state<ItemToggles>({ ...allOn })
  let propertyFilters = $state<PropertyFilter[]>([])
  let result = $state<Evaluation | null>(null)
  // What the written loot filter does with the item (null while unknown).
  let explain = $state<FilterExplanation | null>(null)
  // What of the item can go to the Exotic group (base, named modifiers).
  let exoticCandidates = $state<ExoticCandidate[]>([])
  // "hide" when the item came with the hide shortcut (Alt+H).
  let mode = $state('')

  // The item's worth for the filter's "hidden but valuable" warning: the
  // price list's figure, else the median of the cheapest listings found.
  const worthEx = $derived.by(() => {
    if (!item) return 0
    if (quote) return quote.valueEx * Math.max(1, item.stackSize || 1)
    const e = explain
    if (!e || !result?.listings?.length) return 0
    const rate = (c: string) => (c === 'exalted' ? 1 : c === 'divine' ? e.divineEx : c === 'chaos' ? e.chaosEx : 0)
    const values = result.listings.slice(0, 5).map((l) => l.amount * rate(l.currency)).filter((v) => v > 0).sort((a, b) => a - b)
    return values.length ? values[Math.floor((values.length - 1) / 2)] : 0
  })
  let searched = $state<string[]>([])
  let loading = $state(false)
  let error = $state('')
  let exact = $state(true)
  let status = $state('securable')
  let currency = $state('')
  let indexed = $state('')
  let unidentified = $state('')
  let fractured = $state('')
  let corrupted = $state('')
  let twiceCorrupted = $state('')
  let mirrored = $state('')
  let sanctified = $state('')
  // Modifiable (as POE2 Overlay calls it): an item that is not corrupted,
  // mirrored or sanctified is priced against items that are not either.
  // Corrupted copies of a crafting base sell for a fraction and would
  // otherwise top the list.
  let modifiable = $state(true)
  const canModify = $derived(!!item && !item.stackSize && !item.corrupted && !item.twiceCorrupted && !item.mirrored && !item.sanctified)
  let useItemLevel = $state(false)
  let itemLevelMin = $state<number | undefined>(undefined)
  let itemLevelMax = $state<number | undefined>(undefined)
  let useQuality = $state(false)
  let qualityMin = $state<number | undefined>(undefined)
  let qualityMax = $state<number | undefined>(undefined)
  let useRequiredLevel = $state(false)
  let requiredLevelMin = $state<number | undefined>(undefined)
  let requiredLevelMax = $state<number | undefined>(undefined)
  let evaluateTimer: ReturnType<typeof setTimeout> | undefined
  let hotkey = $state('Alt+E')

  type MetaFilterID = 'itemLevel' | 'quality' | 'requiredLevel'
  type MetaFilterField = 'enabled' | 'min' | 'max'
  type StateFilterID = 'fractured' | 'corrupted' | 'twiceCorrupted' | 'mirrored' | 'sanctified' | 'unidentified'

  const unidentifiedCandidates = $derived.by(() => {
    if (!item || !needsUniqueSelection(item)) return []
    const seen = new Set<string>()
    return (catalog?.items ?? []).flatMap((group) => group.entries ?? []).filter((entry) => {
      if (!entry.name || entry.type !== item?.baseType || seen.has(entry.name)) return false
      seen.add(entry.name)
      return true
    })
  })

  // Unique art by name (from the price snapshot), so the candidates of an
  // unidentified unique can be told apart by their look, as in game.
  let uniqueIcons = $state<Record<string, string | undefined>>({})
  $effect(() => {
    if (!unidentifiedCandidates.length) return
    AppService.UniqueIcons().then((icons) => { uniqueIcons = icons ?? {} }).catch(() => {})
  })

  function needsUniqueSelection(value: Item): boolean {
    return value.unidentified && value.rarity === 'unique' && !value.name
  }

  // Boss keys, splinters, omens and other bulk items trade only on the
  // in-game currency exchange (Ange), which no API shows; the few trade site
  // listings are face-to-face offers and say nothing about the price. The
  // parser marks them; they get the price list's worth card alone, in a
  // window fitted to it, and no search.
  const exchangeId = $derived(item?.exchange ?? '')

  // Fit the window to the worth card. Page pixels: the window's own scale
  // multiplies them on the Go side.
  let compactEl = $state<HTMLElement | null>(null)
  $effect(() => {
    const el = compactEl
    if (!el) return
    // offsetHeight stays in page pixels when the page zooms itself (uiZoom).
    const fit = () => AppService.FitOverlay(el.offsetHeight + 37 + 22).catch(() => {})
    const observer = new ResizeObserver(fit)
    observer.observe(el)
    return () => observer.disconnect()
  })

  function accept(snap: Snapshot) {
    error = snap.error ?? ''
    result = null
    quote = null
    quoteChecked = false
    mode = snap.mode ?? ''
    if (!snap.item) {
      item = null
      choices = []
      return
    }
    exact = true
    item = snap.item
    loadQuote(snap.item)
    loadExplain(snap.item)
    choices = choicesFor(snap.item, false)
    toggles = { ...allOn }
    propertyFilters = propertyFiltersFor(snap.item)
    unidentified = snap.item.unidentified ? 'true' : ''
    fractured = snap.item.fractured ? 'true' : ''
    corrupted = snap.item.corrupted ? 'true' : ''
    twiceCorrupted = snap.item.twiceCorrupted ? 'true' : ''
    mirrored = snap.item.mirrored ? 'true' : ''
    sanctified = snap.item.sanctified ? 'true' : ''
    modifiable = true
    // A waystone's tier (its base) sets what drops; its item level does not.
    // Tablets and uniques are priced by their modifiers, not by item level.
    // The filter is still shown, just not ticked.
    useItemLevel = snap.item.itemLevel > 0 && !['Waystones', 'Tablet', 'Tablets'].includes(snap.item.class) && snap.item.rarity?.toLowerCase() !== 'unique'
    itemLevelMin = snap.item.itemLevel || undefined
    itemLevelMax = undefined
    useQuality = snap.item.quality > 0
    qualityMin = snap.item.quality || undefined
    qualityMax = undefined
    // A gem's requirement follows its level, which is searched instead. The
    // requirement matters to levelling characters: from level 59 up (endgame
    // gear) it is offered but not ticked.
    useRequiredLevel = snap.item.requiredLevel > 0 && snap.item.requiredLevel < 59 && !isGem(snap.item)
    requiredLevelMin = undefined
    requiredLevelMax = snap.item.requiredLevel || undefined
    // The hide window only needs the filter's answer, not a trade search.
    if (!needsUniqueSelection(snap.item) && mode !== 'hide') queueEvaluate(40)
  }


  function loadExplain(value: Item) {
    explain = null
    exoticCandidates = []
    const raw = value.raw
    AppService.ExplainItem(raw).then((e) => {
      if (item?.raw === raw) explain = e
    }).catch(() => {})
    AppService.ExoticCandidates(raw).then((list) => {
      if (item?.raw === raw) exoticCandidates = list ?? []
    }).catch(() => {})
  }

  function loadQuote(value: Item) {
    // Lineage support gems trade on the currency exchange, so the item search
    // finds none; the price list has them. Other gems simply are not found.
    if (!(value.stackSize > 0 || value.rarity === 'currency' || isGem(value) || value.exchange)) return
    // item is a state proxy, so compare the copied text rather than identity.
    const raw = value.raw
    AppService.QuoteCurrency(value.baseType || value.name).then((q) => {
      if (item?.raw !== raw) return
      if (q.found) quote = q
      quoteChecked = true
    }).catch(() => { if (item?.raw === raw) quoteChecked = true })
  }

  onMount(() => {
    Promise.all([AppService.GetTradeCatalog(), AppService.GetOverlaySnapshot()]).then(([loadedCatalog, snap]) => {
      catalog = loadedCatalog
      accept(snap)
    }).catch((e) => (error = cleanError(e)))
    const off = Events.On('overlay-item', (event) => accept(event.data as Snapshot))
    const offLang = followAppLanguage()
    AppService.GetOverlaySettings().then((s) => { if (s?.hotkey) hotkey = s.hotkey }).catch(() => {})
    const openTrade = (event: Event) => AppService.OpenTradePage((event as CustomEvent<string>).detail)
    window.addEventListener('open-trade', openTrade)
    return () => {
      off()
      offLang()
      window.removeEventListener('open-trade', openTrade)
    }
  })

  function selectedFilters(): SelectedFilter[] {
    const out: SelectedFilter[] = []
    if (unidentified) out.push({ group: 'misc_filters', id: 'identified', option: unidentified === 'true' ? 'false' : 'true' })
    if (fractured) out.push({ group: 'misc_filters', id: 'fractured_item', option: fractured })
    if (corrupted) out.push({ group: 'misc_filters', id: 'corrupted', option: corrupted })
    if (twiceCorrupted) out.push({ group: 'misc_filters', id: 'twice_corrupted', option: twiceCorrupted })
    if (mirrored) out.push({ group: 'misc_filters', id: 'mirrored', option: mirrored })
    if (sanctified) out.push({ group: 'misc_filters', id: 'sanctified', option: sanctified })
    if (canModify && modifiable) {
      out.push(...modifiableFilters)
    }
    for (const prop of propertyFilters) {
      if (prop.enabled) out.push({ group: prop.group, id: prop.id, min: prop.min, max: prop.max })
    }
    if (useItemLevel) out.push({ group: 'type_filters', id: 'ilvl', min: itemLevelMin, max: itemLevelMax })
    if (useQuality) out.push({ group: 'type_filters', id: 'quality', min: qualityMin, max: qualityMax })
    if (useRequiredLevel) out.push({ group: 'req_filters', id: 'lvl', min: requiredLevelMin, max: requiredLevelMax })
    if (currency) out.push({ group: 'trade_filters', id: 'price', option: currency })
    if (indexed) out.push({ group: 'trade_filters', id: 'indexed', option: indexed })
    return out
  }

  function queueEvaluate(delay = 320) {
    clearTimeout(evaluateTimer)
    if (exchangeId) return
    evaluateTimer = setTimeout(() => evaluate(false), delay)
  }

  async function evaluate(refresh: boolean) {
    if (!item || loading) return
    loading = true
    error = ''
    try {
      const request = buildRequest(item, choices, status, selectedFilters(), [], toggles)
      searched = searchedStats(request)
      result = await AppService.EvaluateOverlay(request, refresh)
    } catch (e) {
      error = cleanError(e)
      result = null
    } finally {
      loading = false
    }
  }

  function setMode(next: boolean) {
    exact = next
    resetChoiceRanges(choices, !exact)
    resetPropertyRanges(propertyFilters, !exact)
    markDirty()
  }

  function toggleItemPart(part: 'name' | 'base') {
    const next = { ...toggles, [part]: !toggles[part] }
    // A search needs an anchor: keep the base type when neither the unique
    // name nor an item category can stand in for it.
    const namedUnique = next.name && item?.rarity === 'unique'
    if (!next.base && !namedUnique && !categoryFor(item?.class ?? '')) next.base = true
    toggles = next
    markDirty()
  }

  function changeRarity(value: string) {
    toggles = { ...toggles, rarity: value }
    markDirty()
  }

  function updatePropertyFilter(index: number, field: 'enabled' | 'min' | 'max', value: boolean | number | undefined) {
    const prop = propertyFilters[index]
    if (!prop) return
    if (field === 'enabled') prop.enabled = Boolean(value)
    else if (field === 'min') prop.min = value as number | undefined
    else prop.max = value as number | undefined
    markDirty()
  }

  function markDirty() {
    result = null
    error = ''
  }

  function updateMetaFilter(id: MetaFilterID, field: MetaFilterField, value: boolean | number | undefined) {
    if (id === 'itemLevel') {
      if (field === 'enabled') useItemLevel = Boolean(value)
      else if (field === 'min') itemLevelMin = value as number | undefined
      else itemLevelMax = value as number | undefined
    } else if (id === 'quality') {
      if (field === 'enabled') useQuality = Boolean(value)
      else if (field === 'min') qualityMin = value as number | undefined
      else qualityMax = value as number | undefined
    } else {
      if (field === 'enabled') useRequiredLevel = Boolean(value)
      else if (field === 'min') requiredLevelMin = value as number | undefined
      else requiredLevelMax = value as number | undefined
    }
    markDirty()
  }

  function updateStateFilter(id: StateFilterID, value: string) {
    if (id === 'unidentified') unidentified = value
    else if (id === 'fractured') fractured = value
    else if (id === 'corrupted') corrupted = value
    else if (id === 'twiceCorrupted') twiceCorrupted = value
    else if (id === 'mirrored') mirrored = value
    else sanctified = value
    markDirty()
  }

  function chooseUnidentifiedUnique(entry: ItemEntry) {
    if (!item || !entry.name) return
    item = { ...item, name: entry.name }
    result = null
    error = ''
    queueEvaluate(40)
  }

  function cleanError(value: unknown) {
    return String(value).replace(/^RuntimeError:\s*/i, '')
  }

  function openMarket() {
    if (!item || needsUniqueSelection(item)) return
    // The market edits the full item, so it always receives the name and base.
    AppService.ShowMarketWithQuery(buildRequest(item, choices, status, selectedFilters(), [], { ...allOn, rarity: toggles.rarity }))
  }
</script>

<main class="overlay-shell">
  <header>
    <img class="mark" src="/emblem.png" alt="" />
    <strong>MrW Overlay</strong>
    {#if item}<span class="league">{mode === 'hide' ? t('hide.title') : t('ov.priceCheck', item.rarity)}</span>{/if}
    <QuotaBadge />
    <button title={t('craft.open')} aria-label={t('craft.open')} onclick={() => AppService.ShowCraftFromOverlay()}>⚒</button>
    <button title={t('ov.openMarket')} onclick={openMarket}>▣</button>
    <button title={t('window.close')} onclick={() => AppService.HideOverlay()}>×</button>
  </header>

  {#if item}
    <div class="body">
      {#if mode === 'hide'}
        <div bind:this={compactEl}>
          <HidePanel {item} {quote} {explain} {catalog} />
        </div>
      {:else if needsUniqueSelection(item)}
        <section class="unique-picker">
          <small>Unidentified · {item.baseType}</small>
          <h2>{t('ov.whichItem')}</h2>
          {#if unidentifiedCandidates.length}
            <div class="unique-options">
              {#each unidentifiedCandidates as candidate (`${candidate.name}-${candidate.type}`)}
                <button type="button" onclick={() => chooseUnidentifiedUnique(candidate)}>
                  {#if candidate.name && uniqueIcons[candidate.name]}<img src={uniqueIcons[candidate.name]} alt="" />{/if}
                  <strong>{candidate.name}</strong><span>{candidate.type}</span>
                </button>
              {/each}
            </div>
          {:else}
            <p>{t('ov.noUnique')}</p>
          {/if}
        </section>
      {:else}
        {#if exchangeId}
        <div bind:this={compactEl}>
          {#if quote}
          <CurrencyCard {item} {quote} />
          {:else}
          <section class="exchange-title"><strong>{item.baseType || item.name}</strong>{#if quoteChecked}<p>{t('ov.ex.unpriced')}</p>{/if}</section>
          {/if}
          {#if explain}<FilterVerdict {explain} {worthEx} />{/if}
        </div>
        {:else if quote}
        <CurrencyCard {item} {quote} />
        {:else}
        <OverlayItemCard
          {item}
          {choices}
          compact
          onchange={markDirty}
          metaFilters={{
            itemLevel: { enabled: useItemLevel, min: itemLevelMin, max: itemLevelMax },
            quality: { enabled: useQuality, min: qualityMin, max: qualityMax },
            requiredLevel: { enabled: useRequiredLevel, min: requiredLevelMin, max: requiredLevelMax },
          }}
          onmetachange={updateMetaFilter}
          stateFilters={{ unidentified, fractured, corrupted, twiceCorrupted, mirrored, sanctified }}
          modifiable={canModify ? modifiable : undefined}
          onmodifiable={() => { modifiable = !modifiable; markDirty() }}
          onstatechange={updateStateFilter}
          {toggles}
          ontoggle={toggleItemPart}
          onrarity={changeRarity}
          {propertyFilters}
          onpropertychange={updatePropertyFilter}
        />
        {/if}
        {#if explain && !exchangeId}<FilterVerdict {explain} {worthEx} />{/if}
        {#if exoticCandidates.length && !exchangeId}<ExoticAdd candidates={exoticCandidates} />{/if}
      {#if !exchangeId}
      <div class="mode-row">
        <button class:on={exact} onclick={() => setMode(true)}><i></i> {t('ov.exact')}</button>
        <button class:on={!exact} onclick={() => setMode(false)}><i></i> {t('ov.broad')}</button>
      </div>
      <div class="filters">
        <select bind:value={currency} onchange={markDirty} title={t('ov.currency')}>
          <option value="">{t('ov.cur.equivalent')}</option>
          <option value="exalted_divine">{t('ov.cur.exaltedDivine')}</option>
          <option value="exalted">Exalted Orb</option>
          <option value="chaos">Chaos Orb</option>
          <option value="divine">Divine Orb</option>
          <option value="annul">Orb of Annulment</option>
        </select>
        <select bind:value={status} onchange={markDirty} title={t('ov.saleType')}>
          <option value="securable">{t('ov.status.securable')}</option>
          <option value="available">{t('ov.status.available')}</option>
          <option value="onlineleague">{t('ov.status.online')}</option>
          <option value="any">{t('ov.any')}</option>
        </select>
        <select bind:value={indexed} onchange={markDirty} title={t('ov.age')}>
          <option value="">{t('ov.age.any')}</option>
          <option value="3hours">{t('ov.age.3hours')}</option>
          <option value="12hours">{t('ov.age.12hours')}</option>
          <option value="1day">{t('ov.age.1day')}</option>
          <option value="3days">{t('ov.age.3days')}</option>
          <option value="1week">{t('ov.age.1week')}</option>
          <option value="1month">{t('ov.age.1month')}</option>
        </select>
      </div>
      <button class="search-button" disabled={loading} onclick={() => evaluate(true)}>{loading ? t('ov.searching') : t('ov.search')}</button>
      <TradeResults {result} {loading} {error} {searched} />
      {/if}
      {/if}
    </div>
  {:else}
    <div class="capture-error">
      <span>◇</span>
      <strong>{t('ov.cantRead')}</strong>
      <p>{error || t('ov.hoverHint', hotkey)}</p>
    </div>
  {/if}
</main>

<style>
  .overlay-shell { height:100%; display:flex; flex-direction:column; border:1px solid var(--line-strong); background:linear-gradient(180deg,rgba(255,255,255,.025),transparent 180px),var(--grain),rgba(12,14,16,.97); }
  header { height:37px; flex:0 0 37px; display:flex; align-items:center; gap:8px; padding:0 8px; border-bottom:1px solid #47483e; background:#151817; --wails-draggable:drag; }
  header .mark { width:20px; height:20px; }
  header strong { color:var(--gold-bright); font-family:var(--serif); font-size:12px; text-transform:uppercase; }
  header .league { color:var(--muted); flex:1; overflow:hidden; white-space:nowrap; text-overflow:ellipsis; }
  header button { --wails-draggable:no-drag; width:26px; height:25px; border:0; background:transparent; color:#a6a89e; font-size:18px; }
  header button:hover { color:var(--gold-bright); background:#252722; }
  .body { min-height:0; overflow:auto; padding:10px; }
  .unique-picker{padding:18px;border:1px solid #4a4030;background:radial-gradient(circle at top,rgba(120,76,25,.12),transparent 60%),#0b0d0d;text-align:center}.unique-picker>small{color:#d54a45;text-transform:uppercase;letter-spacing:.08em}.unique-picker h2{margin:6px 0 14px;color:var(--gold-bright);font:16px var(--serif)}.unique-picker>p{color:var(--muted)}.unique-options{display:grid;grid-template-columns:repeat(auto-fit,minmax(125px,1fr));gap:8px}.unique-options button{padding:14px 8px;border:1px solid #625337;background:#151713;color:var(--gold-bright)}.unique-options button:hover{border-color:#b59b62;background:#24251e}.unique-options img{display:block;width:64px;height:96px;margin:0 auto 8px;object-fit:contain;filter:drop-shadow(0 2px 6px #000)}.unique-options strong,.unique-options span{display:block}.unique-options strong{font:12px var(--serif)}.unique-options span{margin-top:5px;color:var(--muted);font-size:10px}
  .mode-row { display:flex; justify-content:center; gap:20px; padding:7px; border:1px solid #34342e; border-top:0; background:#111311; }
  .mode-row button { border:0; background:none; color:#a7a89e; }
  .mode-row i { display:inline-block; width:11px; height:11px; margin-right:6px; transform:rotate(45deg); border:1px solid #766b4f; }
  .mode-row button.on { color:var(--gold-bright); }.mode-row button.on i{background:var(--gold);box-shadow:inset 0 0 0 3px #151615}
  .filters { display:grid; grid-template-columns:1.05fr 1fr 1fr; gap:6px; padding:8px 0 3px; }
  select { min-width:0; width:100%; padding:7px 22px 7px 7px; border:1px solid #4b473b; border-radius:2px; background:#191b18; color:#c6c2ad; }
  .exchange-title { padding:12px; text-align:center; border:1px solid #4a4030; background:rgba(7,8,9,.88); }
  .exchange-title strong { font-family:var(--serif); letter-spacing:.04em; color:#d7b76d; font-size:14px; }
  .exchange-title p { margin:8px 0 0; color:var(--muted); }
  .search-button{width:100%;margin:6px 0 3px;padding:9px;border:1px solid #8c7b50;background:#171917;color:var(--gold-bright);font-family:var(--serif);font-weight:bold}.search-button:hover{background:#25261f}.search-button:disabled{opacity:.55}
  .capture-error { box-sizing:border-box; width:calc(100% - 40px); min-width:0; max-width:460px; margin:auto; padding:28px 20px; overflow:hidden; text-align:center; border:1px solid #4a4435; background:#121412; }
  .capture-error>span { display:block; color:var(--gold); font-size:34px; }.capture-error strong{display:block;font-family:var(--serif);color:var(--gold-bright);margin:8px}.capture-error p{margin:8px 0 0;color:var(--muted);overflow-wrap:anywhere}
</style>
