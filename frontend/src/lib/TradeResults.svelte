<script lang="ts">
  import { tick } from 'svelte'
  import { AppService } from '../../bindings/poe2filter'
  import type { Evaluation, EvaluatedListing, EvaluatedMod } from '../../bindings/poe2filter/internal/trade/models'
  import { currencyInfo } from './currencies.svelte'
  import { currentLang, t } from './i18n.svelte'
  import { currencyLabel, listedAgo, PAGE_SIZE, propertySortKey, statSortKey, type SortOption, type SortState } from './overlayQuery'

  let { result = null, loading = false, error = '', expanded = false, sort = null, sortOptions = [], onsort, searched = [] }: {
    result?: Evaluation | null; loading?: boolean; error?: string; expanded?: boolean
    // Stat ids the search filtered on; their lines are tinted on each listing.
    searched?: string[]
    // Sorting is offered only where the caller can run a new search for it.
    sort?: SortState | null; sortOptions?: SortOption[]; onsort?: (key: string, label?: string) => void
  } = $props()
  let preview = $state<EvaluatedListing | null>(null)
  const searchedSet = $derived(new Set(searched))
  // Pages fetched after the first one. The search returns up to 100 ids; the
  // rest are fetched ten at a time as the list scrolls, spending fetch quota
  // only (never a search).
  let extra = $state<EvaluatedListing[]>([])
  let cursor = $state(0)
  let moreLoading = $state(false)
  let moreError = $state('')
  let sentinel = $state<HTMLElement | null>(null)
  let observer: IntersectionObserver | null = null
  const rows = $derived([...(result?.listings ?? []), ...extra])
  const ids = $derived(result?.resultIds ?? [])
  const hasMore = $derived(cursor < ids.length)
  // Weapons get a DPS column, as on the trade site.
  const hasDps = $derived(rows.some((row) => row.item.dps > 0))
  // Chips for the properties the listings actually have, so a ring search
  // does not offer DPS and a weapon search does not offer Energy Shield.
  // Everything is sorted by clicking it on a listing (price, item level,
  // properties, affixes), so the bar only names the current sort; clicking it
  // flips the direction.
  const chips = $derived.by(() => {
    if (!onsort || !sort) return []
    const base: SortOption[] = [{ key: 'price', label: t('ov.sortPrice') }, { key: 'ilvl', label: 'Item Level' }]
    const active = [...base, ...PROPERTY_CHIPS, ...sortOptions].find((chip) => chip.key === sort.key)
    return [active ?? { key: sort.key, label: sort.key }]
  })
  const PROPERTY_CHIPS: SortOption[] = [
    { key: 'dps', label: 'DPS' }, { key: 'pdps', label: 'pDPS' }, { key: 'edps', label: 'eDPS' },
    { key: 'aps', label: 'APS' }, { key: 'crit', label: 'Crit' },
    { key: 'ar', label: 'Armour' }, { key: 'ev', label: 'Evasion' }, { key: 'es', label: 'ES' },
    { key: 'spirit', label: 'Spirit' }, { key: 'block', label: 'Block' }, { key: 'ward', label: 'Runic Ward' },
    { key: 'quality', label: 'Quality' },
    { key: 'map_iir', label: 'Item Rarity' }, { key: 'map_packsize', label: 'Pack Size' },
    { key: 'map_rare_monsters', label: 'Monster Rarity' }, { key: 'map_magic_monsters', label: 'Monster Effectiveness' },
    { key: 'map_bonus', label: 'Drop Chance' }, { key: 'map_revives', label: 'Revives' },
  ]

  $effect(() => {
    // A new result (new search or a new sort) starts the list over.
    const fresh = result
    extra = []
    moreError = ''
    cursor = Math.min(fresh?.listings?.length ? PAGE_SIZE : 0, fresh?.resultIds?.length ?? 0)
    preview = null
  })

  $effect(() => {
    const target = sentinel
    if (!target) return
    observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) loadMore()
    }, { rootMargin: '240px 0px' })
    observer.observe(target)
    return () => { observer?.disconnect(); observer = null }
  })

  async function loadMore() {
    if (moreLoading || !result || cursor >= ids.length) return
    const searchId = result.searchId
    const page = ids.slice(cursor, cursor + PAGE_SIZE)
    moreLoading = true
    moreError = ''
    try {
      const listings = await AppService.FetchOverlayListings(searchId, page) ?? []
      if (result?.searchId !== searchId) return
      extra = [...extra, ...listings]
      cursor += page.length
    } catch (e) {
      moreError = String(e).replace(/^RuntimeError:\s*/i, '')
    } finally {
      moreLoading = false
    }
    // A short page can leave the sentinel on screen; observing it again
    // reports it at once, so the list keeps filling until it scrolls.
    if (!moreError && sentinel && observer) {
      await tick()
      observer.unobserve(sentinel)
      observer.observe(sentinel)
    }
  }

  function toggle(row: EvaluatedListing) {
    preview = preview?.id === row.id ? null : row
  }

  type AffixGroup = { key: string; type: string; tier: string; name: string; lines: { mod: EvaluatedMod; text: string }[] }

  // The trade site lists one line per stat, in stat order, naming the affixes
  // behind each line. Grouping lines by affix puts a hybrid's second stat
  // (Trickster's Stun Threshold) under its own affix, and gives each affix of
  // a summed line its share. Lines without affix details stand alone.
  function affixGroups(mods: EvaluatedMod[]): AffixGroup[] {
    const groups: AffixGroup[] = []
    const byKey = new Map<string, AffixGroup>()
    mods.forEach((mod, index) => {
      const parts = mod.parts?.length ? mod.parts : [{ name: mod.name ?? '', tier: mod.tier ?? '', description: mod.description }]
      for (const part of parts) {
        const named = !!(part.tier || part.name)
        const key = named ? `${mod.type}|${part.tier}|${part.name}` : `line-${index}`
        let group = byKey.get(key)
        if (!group) {
          group = { key, type: mod.type, tier: part.tier ?? '', name: part.name ?? '', lines: [] }
          byKey.set(key, group)
          groups.push(group)
        }
        group.lines.push({ mod, text: part.description })
      }
    })
    return groups
  }

  // Hideout travel per listing: 'busy', 'ok', or '!' + the error.
  let travel = $state<Record<string, string>>({})

  async function goToHideout(row: EvaluatedListing) {
    if (!row.hideoutToken) return
    travel[row.id] = 'busy'
    try {
      await AppService.TravelToHideout(row.hideoutToken)
      travel[row.id] = 'ok'
    } catch (e) {
      travel[row.id] = '!' + String(e).replace(/^RuntimeError:\s*/i, '')
    }
  }

  function hideoutTitle(row: EvaluatedListing) {
    const state = travel[row.id]
    if (state?.startsWith('!')) return state.slice(1)
    if (state === 'ok') return t('ov.hideout.sent')
    if (!row.hideoutToken) return t('ov.hideout.notInstant')
    if (!result?.signedIn) return t('ov.hideout.needLogin')
    return t('ov.hideout.go')
  }

  function arrow(key: string) {
    return sort?.key === key ? (sort.dir === 'asc' ? '▲' : '▼') : ''
  }

</script>

{#snippet itemPreview(row: EvaluatedListing)}
  <div class="preview" class:full={expanded}>
    <div class="preview-title">
      {#if row.item.icon}<img src={row.item.icon} alt="" />{/if}
      <div><strong>{row.item.name || row.item.baseType}</strong>{#if row.item.name}<span>{row.item.baseType}</span>{/if}</div>
    </div>
    <div class="item-meta">
      {#if row.item.rarity}<span>Rarity <b>{row.item.rarity}</b></span>{/if}
      {#if row.item.itemLevel}
        {#if onsort}
          <button type="button" class="prop-sort" class:on={sort?.key === 'ilvl'} title={t('ov.sortByIlvl')} onclick={() => onsort?.('ilvl')}>Item Level <b>{row.item.itemLevel}</b> <i>{arrow('ilvl') || '⇅'}</i></button>
        {:else}
          <span>Item Level <b>{row.item.itemLevel}</b></span>
        {/if}
      {/if}
      {#if row.item.sockets}<span class="sockets" title={`${row.item.sockets} Augmentable Sockets`}>{#each Array(row.item.sockets) as _}<i></i>{/each}</span>{/if}
    </div>
    <div class="preview-props">
      {#each row.item.properties ?? [] as prop}
        {@const key = onsort ? propertySortKey(prop.name) : ''}
        {#if key}
          <button type="button" class="prop-sort" class:on={sort?.key === key} title={t('ov.sortByProp')} onclick={() => onsort?.(key)}>{prop.name}{#if prop.value}: <b>{prop.value}</b>{/if} <i>{arrow(key) || '⇅'}</i></button>
        {:else}
          <span>{prop.name}{#if prop.value}: <b>{prop.value}</b>{/if}</span>
        {/if}
      {/each}
    </div>
    <div class="preview-mods">
      {#each affixGroups(row.item.mods ?? []) as group (group.key)}
        <div class="affix" class:type-implicit={group.type === 'implicit'} class:type-fractured={group.type === 'fractured'} class:type-crafted={group.type === 'crafted'} class:type-desecrated={group.type === 'desecrated'} class:type-rune={group.type === 'rune'}>
          {#if group.tier || group.name}<small>{group.tier} {group.name}</small>{/if}
          {#each group.lines as line}
            {@const key = onsort && line.mod.statId ? statSortKey(line.mod.statId) : ''}
            <p class:searched={!!line.mod.statId && searchedSet.has(line.mod.statId)} title={line.mod.parts?.length ? t('ov.summedAffix', line.mod.description) : undefined}>
              {#if key}
                <button type="button" class="mod-sort" class:on={sort?.key === key} title={t('ov.sortByAffix')} onclick={() => onsort?.(key, line.mod.description)}>{line.text} <i>{arrow(key) || '⇅'}</i></button>
              {:else}
                {line.text}
              {/if}
            </p>
          {/each}
        </div>
      {/each}
    </div>
    {#if row.item.unidentified || row.item.fractured || row.item.corrupted || row.item.mirrored || row.item.sanctified}
      <div class="item-states">
        {#if row.item.unidentified}<span class="unidentified">Unidentified</span>{/if}
        {#if row.item.fractured}<span class="fractured">Fractured</span>{/if}
        {#if row.item.corrupted}<span class="corrupted">{row.item.twiceCorrupted ? 'Twice Corrupted' : 'Corrupted'}</span>{/if}
        {#if row.item.mirrored}<span class="mirrored">Mirrored</span>{/if}
        {#if row.item.sanctified}<span class="sanctified">Sanctified</span>{/if}
      </div>
    {/if}
  </div>
{/snippet}

<div class="results-head">
  <span>{loading ? t('ov.searching') : t('ov.results', result?.total ?? 0)}{#if !loading && rows.length && ids.length}<small>&nbsp;· {t('ov.shown', rows.length)}</small>{/if}</span>
  {#if result?.tradeUrl}<button type="button" onclick={() => window.dispatchEvent(new CustomEvent('open-trade', { detail: result!.tradeUrl }))}>pathofexile.com/trade ↗</button>{/if}
</div>
{#if chips.length}
  <div class="sort-bar" class:busy={loading}>
    <span lang={currentLang()}>{t('ov.sort')}</span>
    {#each chips as chip (chip.key)}
      <button type="button" class:on={sort?.key === chip.key} disabled={loading} title={chip.title ?? chip.label} onclick={() => onsort?.(chip.key)}>{chip.label}{#if arrow(chip.key)} <i>{arrow(chip.key)}</i>{/if}</button>
    {/each}
  </div>
{/if}
{#if error}<p class="result-error">{error}</p>{/if}
{#if loading}<div class="loading"><i></i><span></span><i></i></div>{/if}
{#if !loading && rows.length}
  <div class="result-table" class:expanded class:dps={hasDps}>
    {#if !expanded}<div class="table-head"><span></span><span>{t('ov.sortPrice')}</span><span>iLvl</span>{#if hasDps}<span>DPS</span>{/if}<span>{t('ov.col.account')}</span><span>{t('ov.col.listed')}</span></div>{/if}
    {#each rows as row (row.id)}
      {@const coin = currencyInfo(row.currency)}
      <article class="listing-card" class:full={expanded}>
        {#if expanded}{@render itemPreview(row)}{/if}
        <div class="listing" class:on={!expanded && preview?.id === row.id}>
          <button type="button" class="eye" title={expanded ? t('ov.listing') : t('ov.showItem')} onclick={(event) => { event.stopPropagation(); if (!expanded) toggle(row) }}>{expanded ? '●' : '◉'}</button>
          {#if onsort}
            <button type="button" class="price sortable" class:on={sort?.key === 'price'} title={t('ov.sortByPrice', `${row.amount} × ${coin?.text || currencyLabel(row.currency)}`)} onclick={() => onsort?.('price')}>{row.amount}{#if coin?.image}<i>×</i><img src={coin.image} alt={coin.text} />{:else} <small>{currencyLabel(row.currency)}</small>{/if}<em>{arrow('price') || '⇅'}</em></button>
            <button type="button" class="sortable" class:on={sort?.key === 'ilvl'} title={t('ov.sortByIlvl')} onclick={() => onsort?.('ilvl')}>{row.item.itemLevel}<em>{arrow('ilvl')}</em></button>
          {:else}
            <strong class="price" title={`${row.amount} × ${coin?.text || currencyLabel(row.currency)}`}>{row.amount}{#if coin?.image}<i>×</i><img src={coin.image} alt={coin.text} />{:else} <small>{currencyLabel(row.currency)}</small>{/if}</strong>
            <span>{row.item.itemLevel}</span>
          {/if}
          {#if hasDps}<span class="dps" title={row.item.dps ? `pDPS ${row.item.physicalDps} · eDPS ${row.item.elementalDps}` : ''}>{row.item.dps ? Math.round(row.item.dps) : ''}</span>{/if}
          <span class="account">{row.account}</span>
          <span>{listedAgo(row.listed)}</span>
          <button type="button" class="hideout" class:sent={travel[row.id] === 'ok'} class:failed={travel[row.id]?.startsWith('!')} disabled={!row.hideoutToken || !result?.signedIn || travel[row.id] === 'busy'} title={hideoutTitle(row)} onclick={() => goToHideout(row)}>{travel[row.id] === 'busy' ? '…' : travel[row.id] === 'ok' ? '✓' : '↪'}</button>
        </div>
        {#if !expanded && preview?.id === row.id}{@render itemPreview(row)}{/if}
      </article>
    {/each}
  </div>
  {#if moreError}<p class="result-error">{moreError} <button type="button" onclick={loadMore}>{t('ov.retry')}</button></p>{/if}
  {#if hasMore}
    <div class="more" bind:this={sentinel}>{#if moreLoading}<div class="loading"><i></i><span></span><i></i></div>{:else}<button type="button" onclick={loadMore}>{t('ov.more', ids.length - cursor)}</button>{/if}</div>
  {:else if ids.length && result && result.total > ids.length}
    <p class="end">{t('ov.cap', ids.length)}</p>
  {/if}
{:else if !loading && result}
  <p class="empty">{t('ov.noListings')}</p>
{/if}

<style>
  .sockets { display:inline-flex; align-items:center; gap:5px; vertical-align:middle; }
  .sockets i { width:8px; height:8px; transform:rotate(45deg); border:1px solid var(--gold); background:#15130e; box-shadow:0 0 4px rgba(194,174,126,.35); }
  .results-head { display:flex; align-items:center; justify-content:space-between; min-height:31px; color:var(--muted); font-size:11px; }
  .results-head small { color:#6f746c; }
  .sort-bar { display:flex; flex-wrap:wrap; align-items:center; gap:4px; margin:0 0 8px; }
  .sort-bar > span { color:var(--muted); font-size:9px; text-transform:uppercase; letter-spacing:.08em; margin-right:3px; }
  .sort-bar button { max-width:220px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; padding:4px 8px; border:1px solid #45443a; border-radius:2px; background:#181a17; color:#b6ad93; font-size:10px; }
  .sort-bar button:hover:not(:disabled) { background:#24261f; color:#e7d8a8; }
  .sort-bar button.on { border-color:var(--gold); color:var(--gold-bright); background:#2a281f; }
  .sort-bar button i { font-style:normal; font-size:8px; }
  .sort-bar.busy { opacity:.6; }
  .item-meta .prop-sort { text-transform:inherit; letter-spacing:inherit; }
  .mod-sort { border:0; padding:0 3px; background:none; color:inherit; font:inherit; white-space:pre-line; cursor:pointer; border-radius:2px; }
  .mod-sort i { font-style:normal; font-size:8px; opacity:0; color:#8a7a52; }
  .mod-sort:hover { background:#1b1f28; }.mod-sort:hover i { opacity:1; }
  .mod-sort.on { color:#e0c98f; }.mod-sort.on i { opacity:1; color:var(--gold-bright); }
  .prop-sort { border:0; padding:0 3px; background:none; color:inherit; font:inherit; cursor:pointer; border-radius:2px; }
  .prop-sort i { font-style:normal; font-size:8px; color:#5f6a86; }
  .prop-sort:hover { background:#1b1f28; color:#b4c0e3; }
  .prop-sort.on { color:#d9c07a; }.prop-sort.on i { color:var(--gold-bright); }
  .more { display:flex; justify-content:center; padding:10px 0; }
  .more button,.result-error button { padding:5px 12px; border:1px solid #56523f; background:#181a17; color:#b8ae91; font-size:10px; }
  .end { margin:10px 0; text-align:center; color:#6f746c; font-size:10px; }
  .results-head button { border:0; background:none; color:var(--gold); padding:4px; }
  .result-error { margin:7px 0; padding:8px; color:#e88b84; border:1px solid rgba(192,86,79,.5); background:rgba(90,20,20,.2); user-select:text; }
  .result-table { border:1px solid #34332e; background:#0d0f10; }
  .result-table.expanded { display:grid; gap:9px; border:0; background:transparent; }
  .listing-card.full { overflow:hidden; border:1px solid #4a4030; background:#090a0b; box-shadow:inset 0 0 28px #000; }
  .table-head { display:grid; grid-template-columns:27px 1.1fr 42px 1fr 48px; gap:5px; align-items:center; width:100%; }
  .listing { display:grid; grid-template-columns:23px minmax(68px,1.1fr) 30px minmax(46px,1fr) 38px 25px; gap:4px; align-items:center; width:100%; }
  .dps .table-head { grid-template-columns:27px 1.1fr 42px 42px 1fr 48px; }
  .dps .listing { grid-template-columns:23px minmax(68px,1.1fr) 30px 36px minmax(46px,1fr) 38px 25px; }
  .listing .dps { color:#e0c98f; }
  .table-head { padding:7px 6px; color:#aeb8c2; background:#252824; font-size:11px; }
  .listing { border:0; border-top:1px solid #20221f; padding:6px 5px; text-align:left; color:#aeb4b9; background:#0d0f10; font-size:9px; }
  .listing:hover,.listing.on { background:#171a19; }
  .listing-card.full .listing { border-top:1px solid #57472d; background:#20221e; }
  .listing-card.full .listing:hover { background:#292b25; }
  .listing strong { color:#e7d8a8; white-space:nowrap; }
  .listing small { color:#b7a575; font-size:9px; }
  .listing .price { display:inline-flex; align-items:center; gap:2px; }
  .listing .price i { font-style:normal; color:#8f866c; font-weight:normal; font-size:9px; }
  .listing .price img { width:20px; height:20px; object-fit:contain; margin:-3px 0; }
  .listing .sortable { min-width:0; padding:1px 3px; border:0; border-radius:2px; background:none; color:#aeb4b9; font:inherit; text-align:left; cursor:pointer; white-space:nowrap; }
  .listing .price.sortable { color:#e7d8a8; font-weight:bold; }
  .listing .sortable em { font-style:normal; font-size:8px; margin-left:2px; color:#6f6a5a; opacity:0; }
  .listing .sortable:hover { background:#2b2d27; }.listing .sortable:hover em,.listing .sortable.on em { opacity:1; }
  .listing .sortable.on em { color:var(--gold-bright); }
  .eye,.hideout { min-width:0;height:22px;padding:0;color:#a99c72;border:1px solid #56523f;text-align:center;border-radius:2px;background:#181a17; }
  .hideout { color:#d6ccb0;font-size:15px;background:#696855; }
  .hideout:hover:not(:disabled) { background:#89856d;color:#fff; }.hideout:disabled{opacity:.3}
  .hideout.sent { background:#4d6b3f; color:#e4f5d6; }
  .hideout.failed { background:#7a3a34; color:#ffe1dd; }
  .account { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:#a6b887; }
  .preview { padding:10px 12px 12px; border-top:1px solid #594527; background:radial-gradient(circle at top,rgba(120,76,25,.11),transparent 55%),#090a0b; text-align:center; }
  .preview.full { padding:13px 14px 14px; border-top:0; }
  .preview-title { display:flex; justify-content:center; align-items:center; gap:10px; color:#d7b76d; font-family:var(--serif); }
  .preview-title img { width:42px; height:42px; object-fit:contain; }
  .preview-title strong,.preview-title span { display:block; }
  .preview.full .preview-title strong { color:#e2bd66; font-size:16px; letter-spacing:.035em; }
  .preview.full .preview-title span { color:#c88a42; }
  .item-meta { display:flex; justify-content:center; gap:14px; margin:7px 0 3px; color:var(--muted); font-size:9px; text-transform:uppercase; }
  .item-meta b { color:#d7d2c0; }
  .preview-props { display:flex; flex-direction:column; align-items:center; gap:1px; color:#8493b9; font-size:10px; margin:6px 0; text-align:center; }
  .preview-mods { padding-top:3px; }
  .preview p { margin:4px 0; color:#9aa8d2; font-size:11px; white-space:pre-line; }
  .preview p.searched { background:linear-gradient(90deg,transparent,rgba(122,138,214,.16) 18%,rgba(122,138,214,.16) 82%,transparent); }
  .preview .affix { margin:4px 0; }
  .preview .affix p { margin:0; }
  .preview .affix small { display:block; color:#9d76b6; text-transform:uppercase; font-size:9px; }
  .preview .type-implicit p { color:#7188c4; }
  .preview .type-fractured p { color:#9ed0d8; }
  .preview .type-crafted p { color:#9d76b6; }
  .preview .type-desecrated p { color:#d68869; }
  .preview .type-rune p { color:#7e899d; }
  /* Item states as small badges, in the colours the game uses for them. */
  .item-states { display:flex;flex-wrap:wrap;justify-content:center;gap:5px;margin-top:8px; }
  .item-states span { padding:1px 6px;border:1px solid currentColor;border-radius:2px;font-size:8.5px;line-height:14px;text-transform:uppercase;letter-spacing:.06em;background:rgba(0,0,0,.35); }
  .item-states .unidentified,.item-states .corrupted { color:#d54a45; }
  .item-states .fractured { color:#9ed0d8; }
  .item-states .mirrored { color:#8fa8e6; }
  .item-states .sanctified { color:#d7bd74; }
  .loading { display:flex; justify-content:center; gap:5px; padding:18px; }
  .loading i,.loading span { width:7px; height:7px; transform:rotate(45deg); background:var(--gold-dim); animation:pulse 1s infinite alternate; }
  .loading span { animation-delay:.2s; }.loading i:last-child{animation-delay:.4s}
  .empty { text-align:center; color:var(--muted); padding:16px; border:1px solid #2b2d2b; }
  @keyframes pulse { to { background:var(--gold-bright); } }
</style>
