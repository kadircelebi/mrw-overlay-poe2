<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter'
  import type { Catalog, Item, ItemEntry, SavedSearch, Snapshot, StatEntry, TradeFilter } from '../bindings/poe2filter/internal/overlay/models'
  import type { EvaluateRequest, Evaluation, SelectedFilter, SelectedStat, SelectedStatGroup } from '../bindings/poe2filter/internal/trade/models'
  import TradeResults from './lib/TradeResults.svelte'
  import QuotaBadge from './lib/QuotaBadge.svelte'
  import { allOn, buildRequest, categoryFor, choicesFor, nextSort, searchLabel, searchedStats, statSortKey, type ModChoice, type SortOption, type SortState } from './lib/overlayQuery'

  type FilterState = { min?: number; max?: number; option?: string; input?: string }
  type StatGroupState = { key: number; type: string; min?: number; max?: number; choiceKeys: string[]; weights: Record<string, number | undefined> }

  // A fresh search starts from an empty item: the item box, stats and filters
  // are then filled by hand, and whatever is filled goes into the query.
  function blankItem(): Item {
    return {
      raw: '', class: '', rarity: '', name: '', baseType: '',
      itemLevel: 0, requiredLevel: 0, quality: 0, runeSockets: 0, exceptional: false, stackSize: 0, unidentified: false, fractured: false, corrupted: false, twiceCorrupted: false, mirrored: false, sanctified: false, properties: [], mods: [],
    }
  }

  let catalog = $state<Catalog | null>(null)
  let item = $state<Item | null>(blankItem())
  let choices = $state<ModChoice[]>([])
  let statGroups = $state<StatGroupState[]>([])
  let result = $state<Evaluation | null>(null)
  let searched = $state<string[]>([])
  let loading = $state(false)
  let error = $state('')
  let itemQuery = $state('')
  let statQuery = $state('')
  let showItemSuggestions = $state(false)
  let showStatSuggestions = $state(false)
  let filters = $state<Record<string, FilterState>>({})
  let status = $state('securable')
  let showAdvanced = $state(true)
  let savedCollapsed = $state(false)
  let savedSearches = $state<SavedSearch[]>([])
  let saveName = $state('')
  let saveError = $state('')
  let nextGroupKey = 1
  const defaultSort: SortState = { key: 'price', dir: 'asc' }
  let sort = $state<SortState>({ ...defaultSort })

  const itemEntries = $derived((catalog?.items ?? []).flatMap((group) => group.entries ?? []))
  const statEntries = $derived((catalog?.stats ?? []).flatMap((group) => group.entries ?? []))
  const itemSuggestions = $derived.by(() => {
    const q = itemQuery.trim().toLocaleLowerCase()
    if (q.length < 2) return []
    return itemEntries.filter((entry) => `${entry.name ?? ''} ${entry.type}`.toLocaleLowerCase().includes(q)).slice(0, 18)
  })
  const statSuggestions = $derived.by(() => {
    const q = statQuery.trim().toLocaleLowerCase()
    if (q.length < 2) return []
    return statEntries.filter((entry) => entry.text.toLocaleLowerCase().includes(q)).slice(0, 24)
  })
  const statGroupTypes = [
    ['and', 'And'], ['not', 'Not'], ['if', 'If'], ['count', 'Count'],
    ['weight', 'Weighted Sum'], ['weight2', 'Weighted Sum v2'], ['skill', 'Mercenary Skill Group'],
  ] as const

  // Stats can be sorted by only while the query filters on them, as on the
  // trade site; each selected stat offers itself as a sort chip.
  const statSortOptions = $derived.by((): SortOption[] => {
    const seen = new Set<string>()
    const out: SortOption[] = []
    for (const group of statGroups) for (const key of group.choiceKeys) {
      const choice = choiceForKey(key)
      const sortKey = choice?.selected ? statSortKey(choice.mod.statId) : ''
      if (!sortKey || seen.has(sortKey)) continue
      seen.add(sortKey)
      out.push({ key: sortKey, label: shortStat(choice!.mod.text), title: choice!.mod.text })
    }
    return out
  })

  // A sort label names the stat, not this item's roll: "#% increased Armour".
  function shortStat(text: string) {
    const label = text.replace(/\n/g, ' ').replace(/\([^()]*\)/g, '').replace(/[+-]?\d+(\.\d+)?/g, '#')
    return label.length > 34 ? `${label.slice(0, 32)}…` : label
  }

  // A stat picked from a listing's affix need not be in the query (GGG sorts
  // by any stat); it keeps a chip of its own while it is the sort.
  let pickedStat = $state<SortOption | null>(null)
  const sortOptions = $derived(pickedStat && !statSortOptions.some((option) => option.key === pickedStat!.key)
    ? [...statSortOptions, pickedStat]
    : statSortOptions)

  // A new sort is a new search (GGG sorts all listings, not just the ones
  // shown). The cache still answers a sort that was run a moment ago.
  function setSort(key: string, label?: string) {
    if (loading) return
    sort = nextSort(sort, key)
    if (key.startsWith('stat.') && label && !statSortOptions.some((option) => option.key === key)) {
      const text = label.replace(/\n/g, ' ')
      pickedStat = { key, label: shortStat(text.replace(/[+-]?\d+(\.\d+)?/g, '#')), title: text }
    } else if (!key.startsWith('stat.') || statSortOptions.some((option) => option.key === key)) {
      pickedStat = null
    }
    if (canSearch) search(false)
  }

  function groupNeedsMin(type: string) { return type === 'count' || type === 'weight' || type === 'weight2' }

  // Rarity alone does not narrow a search; anything else the user filled does.
  const canSearch = $derived.by(() => {
    if (!item) return false
    if (item.baseType || (item.rarity === 'unique' && item.name)) return true
    if (requestGroups().length) return true
    return activeFilters().some((filter) => !(filter.group === 'type_filters' && filter.id === 'rarity'))
  })

  function newSearch() {
    item = blankItem()
    choices = []
    statGroups = [{ key: nextGroupKey++, type: 'and', choiceKeys: [], weights: {} }]
    filters = {}
    status = 'securable'
    sort = { ...defaultSort }
    pickedStat = null
    itemQuery = ''
    statQuery = ''
    saveName = ''
    saveError = ''
    showItemSuggestions = false
    showStatSuggestions = false
    result = null
    error = ''
  }

  function accept(snap: Snapshot, draft?: EvaluateRequest) {
    if (!snap.item) return
    const selectedItem = draft?.baseType === snap.item.baseType
      ? { ...snap.item, name: draft.name || snap.item.name, baseType: draft.baseType, rarity: draft.rarity || snap.item.rarity }
      : snap.item
    item = selectedItem
    sort = { ...defaultSort }
    pickedStat = null
    choices = choicesFor(selectedItem, false)
    statGroups = [{ key: nextGroupKey++, type: 'and', choiceKeys: choices.filter((choice) => choice.mod.statId).map((choice) => choice.mod.key), weights: {} }]
    itemQuery = searchLabel(selectedItem)
    seedTypeFilters(selectedItem.rarity, selectedItem.class)
    // An exceptional item is priced by its extra sockets.
    if (selectedItem.exceptional && selectedItem.runeSockets > 0) {
      filters = { ...filters, [stateKey('equipment_filters', 'rune_sockets')]: { min: selectedItem.runeSockets } }
    }
    result = null
    error = ''
    if (draft?.baseType === snap.item.baseType) {
      applyDraft(draft)
      setTimeout(() => search(false), 0)
    }
  }

  onMount(() => {
    Promise.all([AppService.GetTradeCatalog(), AppService.GetOverlaySnapshot(), AppService.GetOverlayDraft(), AppService.GetSavedOverlaySearches()]).then(([c, snap, draft, saved]) => {
      catalog = c
      savedSearches = saved ?? []
      accept(snap, draft)
    }).catch((e) => (error = String(e)))
    const off = Events.On('overlay-item', (event) => accept(event.data as Snapshot))
    const offQuery = Events.On('overlay-query', async (event) => {
      const snap = await AppService.GetOverlaySnapshot()
      accept(snap, event.data as EvaluateRequest)
    })
    const openTrade = (event: Event) => AppService.OpenTradePage((event as CustomEvent<string>).detail)
    window.addEventListener('open-trade', openTrade)
    return () => { off(); offQuery(); window.removeEventListener('open-trade', openTrade) }
  })

  // The type selects start from the item itself, so they show what is being
  // searched instead of "Any": the rarity select is the searched rarity, and the
  // category comes from the item class the game printed.
  function seedTypeFilters(rarity: string, itemClass: string) {
    const rarityKey = stateKey('type_filters', 'rarity')
    if (!filters[rarityKey]?.option) filters[rarityKey] = { ...filters[rarityKey], option: rarity || undefined }
    const category = categoryFor(itemClass)
    const categoryKey = stateKey('type_filters', 'category')
    if (category && !filters[categoryKey]?.option) filters[categoryKey] = { ...filters[categoryKey], option: category }
  }

  function applyDraft(draft: EvaluateRequest) {
    if (!item || draft.baseType !== item.baseType) return
    status = draft.status || 'securable'
    filters = {}
    for (const filter of draft.filters ?? []) {
      filters[stateKey(filter.group, filter.id)] = { min: filter.min ?? undefined, max: filter.max ?? undefined, option: filter.option || undefined, input: filter.input || undefined }
    }
    for (const choice of choices) choice.selected = false
    const sourceGroups: SelectedStatGroup[] = draft.groups?.length
      ? draft.groups
      : [{ type: 'and', stats: draft.stats ?? [] }]
    const used = new Set<string>()
    const next: StatGroupState[] = []
    for (const source of sourceGroups) {
      const keys: string[] = []
      const weights: Record<string, number | undefined> = {}
      for (const selected of source.stats ?? []) {
        let choice = choices.find((candidate) => candidate.mod.statId === selected.id && !used.has(candidate.mod.key))
        if (!choice) {
          // A stat the item line does not name itself, such as the local or
          // global twin in a count group: it gets a row of its own.
          const stat = statEntries.find((entry) => entry.id === selected.id)
          if (!stat) continue
          choice = { selected: true, mod: { key: `draft-${selected.id}-${choices.length}`, statId: stat.id, text: stat.text, type: stat.type, affix: '', name: '', tier: 0, values: [], selected: true } }
          choices = [...choices, choice]
          // Keep working on the reactive copy the state now holds.
          choice = choices[choices.length - 1]
        }
        used.add(choice.mod.key)
        choice.selected = !selected.disabled
        choice.min = selected.min ?? undefined
        choice.max = selected.max ?? undefined
        keys.push(choice.mod.key)
        weights[choice.mod.key] = selected.weight ?? undefined
      }
      next.push({ key: nextGroupKey++, type: source.type || 'and', min: source.min ?? undefined, max: source.max ?? undefined, choiceKeys: keys, weights })
    }
    statGroups = next.length ? next : [{ key: nextGroupKey++, type: 'and', choiceKeys: [], weights: {} }]
    seedTypeFilters(draft.rarity, item.class)
    result = null
    error = ''
  }

  function chooseItem(entry: ItemEntry) {
    // A copied item's affixes belong to that item; stats picked by hand for a
    // fresh search stay when the base is chosen afterwards.
    const fromGame = !!item?.raw
    const rarityKey = stateKey('type_filters', 'rarity')
    const currentRarity = stateFor('type_filters', 'rarity').option ?? ''
    const rarity = entry.name ? 'unique' : currentRarity === 'unique' || fromGame ? '' : currentRarity
    itemQuery = entry.name || entry.type
    item = { ...blankItem(), rarity, name: entry.name ?? '', baseType: entry.type }
    if (fromGame) {
      choices = []
      statGroups = [{ key: nextGroupKey++, type: 'and', choiceKeys: [], weights: {} }]
    }
    filters = { ...filters, [rarityKey]: { option: rarity || undefined } }
    result = null
    showItemSuggestions = false
  }

  // The same stat may be added again: once in an And group and once in a
  // Weighted Sum or Count group, or twice in one group for an item that rolls
  // it twice (Mageblood's "Legacy of Silver"). Each addition is its own row.
  function addStat(stat: StatEntry) {
    if (!item) return
    const choice: ModChoice = { selected: true, mod: { key: `manual-${stat.id}-${choices.length}`, statId: stat.id, text: stat.text, type: stat.type, affix: '', name: '', tier: 0, values: [], selected: true } }
    choices = [...choices, choice]
    if (!statGroups.length) statGroups = [{ key: nextGroupKey++, type: 'and', choiceKeys: [], weights: {} }]
    const target = statGroups.at(-1)!
    statGroups = statGroups.map((group) => group.key === target.key ? { ...group, choiceKeys: [...group.choiceKeys, choice.mod.key] } : group)
    statQuery = ''
    showStatSuggestions = false
    markDirty()
  }

  function choiceForKey(key: string) { return choices.find((choice) => choice.mod.key === key) }

  function setWeight(groupKey: number, choiceKey: string, raw: string) {
    const value = raw === '' ? undefined : Number(raw)
    statGroups = statGroups.map((group) => group.key === groupKey
      ? { ...group, weights: { ...group.weights, [choiceKey]: Number.isFinite(value) ? value : undefined } }
      : group)
    markDirty()
  }

  function addStatGroup() {
    statGroups = [...statGroups, { key: nextGroupKey++, type: 'and', choiceKeys: [], weights: {} }]
    markDirty()
  }

  function removeStatGroup(key: number) {
    if (statGroups.length === 1) return
    statGroups = statGroups.filter((group) => group.key !== key)
    markDirty()
  }

  function removeChoice(groupKey: number, choiceKey: string) {
    statGroups = statGroups.map((group) => group.key === groupKey ? { ...group, choiceKeys: group.choiceKeys.filter((key) => key !== choiceKey) } : group)
    if (!statGroups.some((group) => group.choiceKeys.includes(choiceKey))) {
      const choice = choiceForKey(choiceKey)
      if (choice) choice.selected = false
    }
    markDirty()
  }

  function markDirty() {
    result = null
    error = ''
  }

  function stateKey(group: string, id: string) { return `${group}.${id}` }
  function stateFor(group: string, id: string): FilterState {
    return filters[stateKey(group, id)] ?? {}
  }

  function setNumber(group: string, id: string, field: 'min' | 'max', raw: string) {
    const value = raw === '' ? undefined : Number(raw)
    const key = stateKey(group, id)
    filters = {
      ...filters,
      [key]: { ...filters[key], [field]: Number.isFinite(value) ? value : undefined },
    }
    markDirty()
  }

  function setOption(group: string, id: string, value: string) {
    const key = stateKey(group, id)
    filters = { ...filters, [key]: { ...filters[key], option: value || undefined } }
    markDirty()
  }

  function setInput(group: string, id: string, value: string) {
    const key = stateKey(group, id)
    filters = { ...filters, [key]: { ...filters[key], input: value.trim() ? value : undefined } }
    markDirty()
  }

  function activeFilters(): SelectedFilter[] {
    const out: SelectedFilter[] = []
    for (const [key, value] of Object.entries(filters)) {
      const dot = key.indexOf('.')
      if (dot < 0) continue
      const input = value.input?.trim()
      if (value.min === undefined && value.max === undefined && !value.option && !input) continue
      out.push({ group: key.slice(0, dot), id: key.slice(dot + 1), min: value.min, max: value.max, option: value.option, input })
    }
    return out
  }

  async function search(refresh = true) {
    if (loading) return
    if (!canSearch) { error = 'Önce bir eşya, stat veya filtre seçin.'; return }
    loading = true; error = ''
    const request = currentRequest()
    searched = searchedStats(request)
    try { result = await AppService.EvaluateOverlay({ ...request, sort: sort.key, sortDir: sort.dir }, refresh) }
    catch (e) { error = String(e).replace(/^RuntimeError:\s*/i, ''); result = null }
    finally { loading = false }
  }

  function currentRequest() {
    return buildRequest(item!, choices, status, activeFilters(), requestGroups(), { ...allOn, rarity: stateFor('type_filters', 'rarity').option ?? '' })
  }

  async function saveCurrent() {
    if (!item || !canSearch) return
    saveError = ''
    const name = saveName.trim() || searchLabel(item) || 'Search'
    try {
      savedSearches = await AppService.SaveOverlaySearch(name, currentRequest()) ?? []
      saveName = ''
    } catch (e) {
      saveError = String(e).replace(/^RuntimeError:\s*/i, '')
    }
  }

  async function deleteSaved(id: string, event: MouseEvent) {
    event.stopPropagation()
    saveError = ''
    try { savedSearches = await AppService.DeleteOverlaySearch(id) ?? [] }
    catch (e) { saveError = String(e).replace(/^RuntimeError:\s*/i, '') }
  }

  function loadSaved(saved: SavedSearch) {
    const query = saved.query
    const selected = (query.groups?.length ? query.groups.flatMap((group) => group.stats ?? []) : query.stats ?? [])
    item = {
      raw: '', class: '', rarity: query.rarity, name: query.name, baseType: query.baseType,
      itemLevel: 0, requiredLevel: 0, quality: 0, runeSockets: 0, exceptional: false, stackSize: 0,
      unidentified: query.filters?.some((filter) => filter.group === 'misc_filters' && filter.id === 'identified' && filter.option === 'false') ?? false,
      fractured: query.filters?.some((filter) => filter.group === 'misc_filters' && filter.id === 'fractured_item' && filter.option === 'true') ?? false,
      corrupted: query.filters?.some((filter) => filter.group === 'misc_filters' && filter.id === 'corrupted' && filter.option === 'true') ?? false,
      twiceCorrupted: query.filters?.some((filter) => filter.group === 'misc_filters' && filter.id === 'twice_corrupted' && filter.option === 'true') ?? false,
      sanctified: query.filters?.some((filter) => filter.group === 'misc_filters' && filter.id === 'sanctified' && filter.option === 'true') ?? false,
      mirrored: query.filters?.some((filter) => filter.group === 'misc_filters' && filter.id === 'mirrored' && filter.option === 'true') ?? false,
      properties: [],
      mods: selected.map((stat, index) => {
        const catalogStat = statEntries.find((entry) => entry.id === stat.id)
        return {
          key: `saved-${index}-${stat.id}`, statId: stat.id,
          text: catalogStat?.text ?? stat.id, type: catalogStat?.type ?? 'explicit',
          affix: '', name: '', tier: 0, values: [], selected: true,
        }
      }),
    }
    choices = choicesFor(item, false)
    statGroups = [{ key: nextGroupKey++, type: 'and', choiceKeys: choices.map((choice) => choice.mod.key), weights: {} }]
    itemQuery = searchLabel(query)
    applyDraft(query)
    saveName = saved.name
  }

  function requestGroups(): SelectedStatGroup[] {
    return statGroups.map((group) => ({
      type: group.type,
      min: finiteOrUndefined(group.min),
      max: finiteOrUndefined(group.max),
      stats: group.choiceKeys.map(choiceForKey).filter((choice): choice is ModChoice => !!choice && choice.selected && !!choice.mod.statId).map((choice): SelectedStat => ({
        id: choice.mod.statId,
        min: group.type === 'weight' || group.type === 'weight2' ? undefined : choice.min,
        max: group.type === 'weight' || group.type === 'weight2' ? undefined : choice.max,
        weight: group.type === 'weight' || group.type === 'weight2' ? group.weights[choice.mod.key] ?? 1 : undefined,
      })),
    })).filter((group) => group.stats.length > 0)
  }

  function filterOptions(filter: TradeFilter) { return filter.option?.options ?? [] }

  // An emptied number box binds null; the query must not carry it.
  function finiteOrUndefined(value: number | null | undefined) { return typeof value === 'number' && Number.isFinite(value) ? value : undefined }
</script>

<main class="market-shell">
  <header>
    <span class="mark">⚖</span><strong>MrW Overlay · Market</strong>
    <span class="league">{(item && searchLabel(item)) || 'Advanced Search'}</span>
    <QuotaBadge />
    <button title="Filtreleri göster/gizle" onclick={() => (showAdvanced = !showAdvanced)}>⌁</button>
    <button title="Kapat" onclick={() => AppService.HideMarket()}>×</button>
  </header>
  <div class="workspace" class:no-advanced={!showAdvanced} class:saved-collapsed={savedCollapsed}>
    <aside class="saved-pane" class:collapsed={savedCollapsed}>
      <button class="saved-toggle" title={savedCollapsed ? 'Kayıtlı aramaları aç' : 'Kayıtlı aramaları kapat'} onclick={() => (savedCollapsed = !savedCollapsed)}>{savedCollapsed ? '›' : '‹'}</button>
      {#if !savedCollapsed}
        <div class="saved-title"><strong>Explorer</strong></div>
        <div class="save-box">
          <input bind:value={saveName} placeholder={(item && searchLabel(item)) || 'Search name'} />
          <button disabled={!canSearch} onclick={saveCurrent}>＋ Save</button>
        </div>
        {#if saveError}<p class="save-error">{saveError}</p>{/if}
        <div class="saved-list">
          <small>Saved Searches</small>
          {#each savedSearches as saved (saved.id)}
            <div class="saved-row">
              <button class="saved-load" onclick={() => loadSaved(saved)}><span>♥</span><b>{saved.name}</b><i>{saved.query.name || saved.query.baseType}</i></button>
              <button class="saved-delete" title="Kaydı sil" onclick={(event) => deleteSaved(saved.id, event)}>×</button>
            </div>
          {:else}
            <p>Henüz kayıt yok.</p>
          {/each}
        </div>
      {/if}
    </aside>
    <section class="search-pane">
      <div class="tabs"><button class="on">Search</button><button disabled>Exchange</button><button disabled>Live Search</button></div>
      <button class="new-search" title="Eşyayı, statları ve filtreleri temizle" onclick={newSearch}>＋ New Search</button>
      <div class="item-search">
        <input bind:value={itemQuery} onfocus={() => (showItemSuggestions = true)} oninput={() => (showItemSuggestions = true)} placeholder="Search items…" spellcheck="false" />
        <button disabled={!canSearch || loading} onclick={() => search(true)}>{loading ? '…' : '⌕'}</button>
        {#if showItemSuggestions && itemSuggestions.length}
          <div class="suggestions">
            {#each itemSuggestions as entry}<button onclick={() => chooseItem(entry)}><b>{entry.name || entry.type}</b>{#if entry.name}<span>{entry.type}</span>{/if}</button>{/each}
          </div>
        {/if}
      </div>
      <button class="search-button" disabled={!canSearch || loading} onclick={() => search(true)}>{loading ? 'Searching…' : 'Search'}</button>
      <TradeResults {result} {loading} {error} {searched} expanded {sort} {sortOptions} onsort={setSort} />
    </section>

    {#if showAdvanced}
      <aside class="advanced">
        <div class="advanced-title"><strong>⌁ Advanced Filters</strong><button onclick={() => (showAdvanced = false)}>×</button></div>
        <div class="advanced-scroll">
          <section class="filter-group">
            <h2>Stat Filters</h2>
            {#each statGroups as statGroup (statGroup.key)}
              <div class="stat-group">
                <div class="stat-group-head">
                  <select bind:value={statGroup.type} onchange={markDirty}>
                    {#each statGroupTypes as option}<option value={option[0]}>{option[1]}</option>{/each}
                  </select>
                  {#if groupNeedsMin(statGroup.type)}<span class="group-range"><input type="number" bind:value={statGroup.min} oninput={markDirty} placeholder="min" title={statGroup.type === 'count' ? 'En az kaç stat' : 'En az toplam'} /><input type="number" bind:value={statGroup.max} oninput={markDirty} placeholder="max" title={statGroup.type === 'count' ? 'En çok kaç stat' : 'En çok toplam'} /></span>{:else}<span></span>{/if}
                  <span>FILTER {statGroups.indexOf(statGroup) + 1}</span>
                  <button disabled={statGroups.length === 1} title="Grubu sil" onclick={() => removeStatGroup(statGroup.key)}>×</button>
                </div>
                {#each statGroup.choiceKeys as choiceKey (choiceKey)}
                  {@const choice = choiceForKey(choiceKey)}
                  {#if choice}
                    <div class="stat-choice" class:off={!choice.selected}>
                      <label><input type="checkbox" bind:checked={choice.selected} onchange={markDirty} /><i></i><span>{choice.mod.text}</span></label>
                      {#if statGroup.type === 'weight' || statGroup.type === 'weight2'}
                        <span class="weight"><input type="number" value={statGroup.weights[choiceKey] ?? 1} oninput={(event) => setWeight(statGroup.key, choiceKey, event.currentTarget.value)} placeholder="weight" /></span>
                      {:else}
                        <span class="minmax"><input type="number" bind:value={choice.min} oninput={markDirty} placeholder="min" /><input type="number" bind:value={choice.max} oninput={markDirty} placeholder="max" /></span>
                      {/if}
                      <button class="stat-sort" class:on={sort.key === statSortKey(choice.mod.statId)} disabled={!choice.selected || !choice.mod.statId || loading} title="Sonuçları bu stata göre sırala" onclick={() => setSort(statSortKey(choice.mod.statId))}>{sort.key === statSortKey(choice.mod.statId) ? (sort.dir === 'asc' ? '▲' : '▼') : '⇅'}</button>
                      <button title="Affixi gruptan çıkar" onclick={() => removeChoice(statGroup.key, choiceKey)}>×</button>
                    </div>
                  {/if}
                {/each}
              </div>
            {/each}
            <div class="stat-add">
              <input bind:value={statQuery} onfocus={() => (showStatSuggestions = true)} oninput={() => (showStatSuggestions = true)} placeholder="+ Add Stat Filter" spellcheck="false" />
              {#if showStatSuggestions && statSuggestions.length}
                <div class="stat-suggestions">
                  {#each statSuggestions as stat}<button onclick={() => addStat(stat)}><small>{stat.type}</small><span>{stat.text}</span></button>{/each}
                </div>
              {/if}
            </div>
            <button class="add-group" onclick={addStatGroup}>＋ Add Stat Group Filter</button>
          </section>
          {#each catalog?.filters ?? [] as group (group.id)}
            {#if group.id !== 'status_filters'}
              <section class="filter-group">
                <h2>{group.title}</h2>
                {#if group.id === 'trade_filters'}
                  <label class="status-line"><span>Search Status</span><select bind:value={status} onchange={markDirty}><option value="securable">Instant Buyout</option><option value="available">Instant + In Person</option><option value="onlineleague">Online in League</option><option value="any">Any</option></select></label>
                {/if}
                {#each group.filters ?? [] as filter (filter.id)}
                  <label class="filter-row">
                    <span>{filter.text}</span>
                    {#if filter.minMax}
                      <span class="filter-controls">
                        {#if filterOptions(filter).length}
                          <select value={stateFor(group.id, filter.id).option ?? ''} onchange={(e) => setOption(group.id, filter.id, e.currentTarget.value)}>
                            {#each filterOptions(filter) as option}<option value={option.id ?? ''}>{option.text}</option>{/each}
                          </select>
                        {/if}
                        <span class="minmax"><input type="number" value={stateFor(group.id, filter.id).min ?? ''} oninput={(e) => setNumber(group.id, filter.id, 'min', e.currentTarget.value)} placeholder="min" /><input type="number" value={stateFor(group.id, filter.id).max ?? ''} oninput={(e) => setNumber(group.id, filter.id, 'max', e.currentTarget.value)} placeholder="max" /></span>
                      </span>
                    {:else if filterOptions(filter).length}
                      <select value={stateFor(group.id, filter.id).option ?? ''} onchange={(e) => setOption(group.id, filter.id, e.currentTarget.value)}>
                        {#each filterOptions(filter) as option}<option value={option.id ?? ''}>{option.text}</option>{/each}
                      </select>
                    {:else}
                      <input value={stateFor(group.id, filter.id).input ?? ''} oninput={(e) => setInput(group.id, filter.id, e.currentTarget.value)} placeholder={filter.input?.placeholder || '…'} spellcheck="false" />
                    {/if}
                  </label>
                {/each}
              </section>
            {/if}
          {/each}
        </div>
      </aside>
    {/if}
  </div>
</main>

<style>
  .market-shell{height:100%;display:flex;flex-direction:column;border:1px solid var(--line-strong);background:var(--grain),#101210}
  header{height:36px;flex:0 0 36px;display:flex;align-items:center;gap:7px;padding:0 8px;border-bottom:1px solid #4b4a3e;background:#151715;--wails-draggable:drag;font-size:11px}header .mark{color:var(--gold);font-size:15px}header strong{font-family:var(--serif);color:var(--gold-bright)}header .league{flex:1;overflow:hidden;text-overflow:ellipsis;color:var(--muted);white-space:nowrap}header button{--wails-draggable:no-drag;border:0;background:none;color:#aaa;font-size:17px}
  .workspace{min-height:0;flex:1;display:grid;grid-template-columns:116px minmax(245px,.9fr) minmax(280px,1.1fr);overflow:hidden}.workspace.saved-collapsed{grid-template-columns:28px minmax(245px,.9fr) minmax(280px,1.1fr)}.workspace.no-advanced{grid-template-columns:116px 1fr}.workspace.no-advanced.saved-collapsed{grid-template-columns:28px 1fr}
  .saved-pane{position:relative;min-width:0;overflow:hidden;border-right:1px solid #3a392f;background:#11130f}.saved-pane.collapsed{background:#20221d}.saved-toggle{position:absolute;z-index:3;top:7px;right:4px;width:19px;height:22px;border:1px solid #45443a;background:#24261f;color:#b8ae91}.saved-title{height:36px;display:flex;align-items:center;padding:0 26px 0 8px;border-bottom:1px solid #38382f;color:#d6ccb0;font-family:var(--serif);font-size:10px}.save-box{display:grid;gap:5px;padding:7px}.save-box input{min-width:0;width:100%;padding:6px;border:1px solid #414139;background:#171916;color:#ccc6b2;font-size:9px}.save-box button{padding:6px;border:1px solid #665b40;background:#20221d;color:var(--gold-bright);font-size:9px}.save-box button:disabled{opacity:.4}.save-error{margin:0 7px 7px;color:#df8179;font-size:8px}.saved-list{height:calc(100% - 98px);overflow:auto;padding:3px 5px 8px}.saved-list>small{display:block;padding:5px 3px;color:#777d76;text-transform:uppercase;font-size:7px}.saved-list>p{padding:6px 3px;color:#6f746f;font-size:8px}.saved-row{position:relative;display:grid;grid-template-columns:1fr 17px;width:100%;margin-bottom:4px;border:1px solid #30322b;background:#191b17}.saved-row:hover{border-color:#756847;background:#23251e}.saved-load{min-width:0;display:grid;grid-template-columns:12px 1fr;gap:2px 3px;padding:6px 3px 6px 5px;border:0;background:none;text-align:left}.saved-load>span{grid-row:1/3;color:#9f9067}.saved-load b{overflow:hidden;text-overflow:ellipsis;color:#d2c8aa;font-size:8px;white-space:nowrap}.saved-load i{overflow:hidden;text-overflow:ellipsis;color:#737a78;font-size:7px;font-style:normal;white-space:nowrap}.saved-delete{padding:0;border:0;border-left:1px solid #2f302a;background:none;color:#817a69;font-size:13px}.saved-delete:hover{color:#df8179}
  .search-pane{position:relative;min-width:0;padding:8px;overflow:auto;border-right:1px solid #37372f}.tabs{display:grid;grid-template-columns:repeat(3,1fr);margin:-8px -8px 8px}.tabs button{padding:8px 3px;border:0;border-bottom:1px solid #34362f;background:#171917;color:#898d85;font-size:9px}.tabs button.on{color:var(--gold-bright);border-bottom:2px solid var(--gold);background:#20221e}.item-search{position:relative;display:grid;grid-template-columns:1fr 34px;gap:5px;margin-bottom:7px}.item-search>input{min-width:0;padding:7px;border:1px solid #46463d;background:#111311;font-size:10px}.item-search>button{border:1px solid #5b543f;background:#20221e;color:var(--gold-bright);font-size:17px}.suggestions,.stat-suggestions{position:absolute;z-index:20;top:100%;left:0;right:39px;max-height:260px;overflow:auto;border:1px solid #555042;background:#111310;box-shadow:0 10px 30px #000}.suggestions button,.stat-suggestions button{display:flex;width:100%;justify-content:space-between;gap:8px;padding:7px;border:0;border-bottom:1px solid #282a25;background:transparent;text-align:left;font-size:9px}.suggestions button:hover,.stat-suggestions button:hover{background:#25271f}.suggestions span{color:var(--muted)}.search-button{position:sticky;bottom:0;width:100%;margin-top:7px;padding:8px;border:1px solid #8c7b50;background:#171917;color:var(--gold-bright);font-family:var(--serif);font-size:10px;font-weight:bold}.search-button:hover{background:#25261f}.search-button:disabled{opacity:.5}
  .advanced{min-width:0;min-height:0;display:flex;flex-direction:column;overflow:hidden;background:#171914}.advanced-title{display:flex;flex:0 0 auto;justify-content:space-between;padding:8px 10px;border-bottom:1px solid #48483c;background:#2b2d27;color:#d8cfb1;font-family:var(--serif);font-size:10px}.advanced-title button{border:0;background:none;color:#999}.advanced-scroll{min-height:0;flex:1 1 auto;overflow-y:auto;overflow-x:hidden;padding:7px}.filter-group{border:1px solid #33352f;margin-bottom:7px;background:#11130f}.filter-group h2{margin:0 0 6px;padding:7px 8px;background:#30322c;color:#ddd4b5;font-family:var(--serif);font-size:10px}.filter-row,.status-line{display:grid;grid-template-columns:minmax(90px,.85fr) minmax(112px,1.15fr);align-items:center;gap:6px;padding:4px 8px;color:#d0c7a9}.filter-row>span:first-child{font-size:9px}.filter-row input,.filter-row select,.status-line select,.stat-add input{min-width:0;width:100%;padding:6px;border:1px solid #414139;border-radius:2px;background:#20221d;color:#ccc6b2;font-size:9px}.minmax{display:grid;grid-template-columns:1fr 1fr;gap:5px}.stat-add{position:relative;margin:7px 8px 9px}.stat-suggestions{right:0;max-height:320px}.stat-suggestions button{justify-content:flex-start}.stat-suggestions small{flex:0 0 52px;color:#77a070;text-transform:uppercase}.stat-suggestions span{color:#c3bda8}
  .stat-group{margin:7px 8px;border:1px solid #34362f;background:#0d0f0d}.stat-group-head{display:grid;grid-template-columns:minmax(72px,105px) minmax(90px,120px) 1fr 22px;gap:5px;align-items:center;padding:5px;border-bottom:1px solid #303229}.stat-group-head select,.stat-group-head input{min-width:0;width:100%;padding:5px;border:1px solid #504b3c;background:#20221d;color:#d7cfb5;font-size:9px}.stat-group-head>span{color:#6f9c6c;font-size:8px;font-weight:bold}.stat-group-head button,.stat-choice>button{border:0;background:none;color:#9b9380;font-size:14px}.stat-group-head button:disabled{opacity:.25}.stat-choice{display:grid;grid-template-columns:minmax(0,1fr) 94px 18px 20px;gap:5px;align-items:center;padding:5px;border-top:1px solid #23251f}.stat-choice.off{opacity:.5}.stat-choice>button.stat-sort{padding:0;font-size:11px;color:#6f6a5a}.stat-choice>button.stat-sort:hover:not(:disabled){color:var(--gold-bright)}.stat-choice>button.stat-sort.on{color:var(--gold-bright)}.stat-choice>button.stat-sort:disabled{opacity:.3}.stat-choice>label{display:flex;gap:6px;align-items:flex-start;min-width:0;color:#aeb9d5;font-size:9px}.stat-choice>label input{position:absolute;opacity:0}.stat-choice>label i{flex:0 0 10px;width:10px;height:10px;margin-top:2px;transform:rotate(45deg);border:1px solid var(--gold-dim)}.stat-choice>label input:checked+i{background:var(--gold);box-shadow:inset 0 0 0 3px #17191b}.stat-choice>label span{overflow-wrap:anywhere}.stat-choice .minmax input,.stat-choice .weight input{min-width:0;width:100%;padding:4px;border:1px solid #3d3a31;background:#111313;color:var(--gold-bright)}.add-group{display:block;margin:0 8px 8px auto;padding:6px 8px;border:1px solid #716342;background:#191b17;color:var(--gold-bright);font-family:var(--serif);font-size:9px}
  .group-range{display:grid;grid-template-columns:1fr 1fr;gap:3px}.filter-controls{display:grid;gap:4px}
  .new-search{display:block;width:100%;margin-bottom:7px;padding:6px;border:1px solid #5b543f;background:#191b17;color:var(--gold-bright);font-family:var(--serif);font-size:9px}.new-search:hover{background:#25261f}
</style>
