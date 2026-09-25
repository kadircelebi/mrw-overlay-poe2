<script lang="ts">
  import type { Item, ItemMod } from '../../bindings/poe2filter/internal/overlay/models'
  import { rarityOptions, type ItemToggles, type ModChoice, type PropertyFilter } from './overlayQuery'

  type MetaFilter = { enabled: boolean; min?: number; max?: number }
  type MetaFilterID = 'itemLevel' | 'quality' | 'requiredLevel'
  type MetaFilterField = 'enabled' | 'min' | 'max'
  type StateFilterID = 'fractured' | 'corrupted' | 'twiceCorrupted' | 'mirrored' | 'sanctified' | 'unidentified'

  let {
    item,
    choices = undefined,
    compact = false,
    onchange = () => {},
    metaFilters = undefined,
    onmetachange = () => {},
    stateFilters = undefined,
    onstatechange = () => {},
    toggles = undefined,
    ontoggle = () => {},
    onrarity = () => {},
    propertyFilters = undefined,
    onpropertychange = () => {},
  }: {
    item: Item
    choices?: ModChoice[]
    compact?: boolean
    onchange?: () => void
    metaFilters?: Record<MetaFilterID, MetaFilter>
    onmetachange?: (id: MetaFilterID, field: MetaFilterField, value: boolean | number | undefined) => void
    stateFilters?: Record<StateFilterID, string>
    onstatechange?: (id: StateFilterID, value: string) => void
    toggles?: ItemToggles
    ontoggle?: (part: 'name' | 'base') => void
    onrarity?: (value: string) => void
    propertyFilters?: PropertyFilter[]
    onpropertychange?: (index: number, field: 'enabled' | 'min' | 'max', value: boolean | number | undefined) => void
  } = $props()

  const qualityProperty = $derived(item.properties?.find((property) => property.name.startsWith('Quality')))
  const otherProperties = $derived(item.properties?.filter((property) => !property.name.startsWith('Quality') && !propertyFilters?.some((filter) => filter.name === property.name)) ?? [])

  function numeric(raw: string): number | undefined {
    if (raw === '') return undefined
    const value = Number(raw)
    return Number.isFinite(value) ? value : undefined
  }

  function label(choice: ModChoice): string {
    const mod = choice.mod
    if (mod.type === 'pseudo') return 'Pseudo'
    const affix = mod.affix ? mod.affix[0].toUpperCase() + mod.affix.slice(1) : mod.type
    const special = ['fractured', 'crafted', 'desecrated'].includes(mod.type)
      ? `${mod.type[0].toUpperCase()}${mod.type.slice(1)} `
      : ''
    return `${special}${affix}${tiers(mod)}`
  }

  // Affixes of the same stat are summed into one line; its label lists every
  // tier ("Prefix T1+T2") as the trade site sees one value.
  function tiers(mod: ItemMod): string {
    if (mod.tiers?.length) return mod.tiers.some((tier) => tier > 0) ? ` ${mod.tiers.map((tier) => tier ? `T${tier}` : '—').join('+')}` : ''
    return mod.tier ? ` T${mod.tier}` : ''
  }
</script>

<article class="item-card" class:compact>
  <div class="item-title" class:unique={item.rarity === 'unique'} class:rare={item.rarity === 'rare'}>
    {#if toggles}
      {#if item.name && item.rarity === 'unique'}
        <button type="button" class="title-toggle" class:off={!toggles.name} title="Aramaya dahil et / çıkar" onclick={() => ontoggle('name')}><strong>{item.name}</strong></button>
      {:else if item.name}<strong>{item.name}</strong>{/if}
      <button type="button" class="title-toggle" class:off={!toggles.base} title="Aramaya dahil et / çıkar" onclick={() => ontoggle('base')}><span>{item.baseType}</span></button>
    {:else}
      {#if item.name}<strong>{item.name}</strong>{/if}
      <span>{item.baseType}</span>
    {/if}
  </div>
  <div class="item-meta">
    <span class="item-class">{item.class}</span>
    {#if toggles}
      <label class="rarity-select" class:off={(toggles.rarity ?? item.rarity) === ''}>
        <span>Rarity</span>
        <select value={toggles.rarity ?? item.rarity} onchange={(event) => onrarity(event.currentTarget.value)}>
          {#each rarityOptions as option (option.id)}<option value={option.id}>{option.text}</option>{/each}
        </select>
      </label>
    {:else if item.rarity}
      <span>{item.rarity}</span>
    {/if}
    {#if metaFilters}
      <div class="meta-filter" class:off={!metaFilters.itemLevel.enabled}>
        <label title="Aramaya dahil et"><input type="checkbox" checked={metaFilters.itemLevel.enabled} onchange={(event) => onmetachange('itemLevel', 'enabled', event.currentTarget.checked)} /><i></i><span>Item Level</span></label>
        <span class="meta-range"><input type="number" value={metaFilters.itemLevel.min ?? ''} oninput={(event) => onmetachange('itemLevel', 'min', numeric(event.currentTarget.value))} placeholder="min" /><input type="number" value={metaFilters.itemLevel.max ?? ''} oninput={(event) => onmetachange('itemLevel', 'max', numeric(event.currentTarget.value))} placeholder="max" /></span>
      </div>
      <div class="meta-filter" class:off={!metaFilters.requiredLevel.enabled}>
        <label title="Aramaya dahil et"><input type="checkbox" checked={metaFilters.requiredLevel.enabled} onchange={(event) => onmetachange('requiredLevel', 'enabled', event.currentTarget.checked)} /><i></i><span>Requires</span></label>
        <span class="meta-range"><input type="number" value={metaFilters.requiredLevel.min ?? ''} oninput={(event) => onmetachange('requiredLevel', 'min', numeric(event.currentTarget.value))} placeholder="min" /><input type="number" value={metaFilters.requiredLevel.max ?? ''} oninput={(event) => onmetachange('requiredLevel', 'max', numeric(event.currentTarget.value))} placeholder="max" /></span>
      </div>
    {:else}
      {#if item.itemLevel}<span>Item Level <b>{item.itemLevel}</b></span>{/if}
      {#if item.requiredLevel}<span>Requires <b>{item.requiredLevel}</b></span>{/if}
    {/if}
  </div>
  {#if metaFilters || item.properties?.length}
    <div class="properties">
      {#if metaFilters}
        <div class="meta-filter quality-filter" class:off={!metaFilters.quality.enabled}>
          <label title="Aramaya dahil et"><input type="checkbox" checked={metaFilters.quality.enabled} onchange={(event) => onmetachange('quality', 'enabled', event.currentTarget.checked)} /><i></i><span>{qualityProperty?.name ?? 'Quality'}</span></label>
          <span class="meta-range"><input type="number" value={metaFilters.quality.min ?? ''} oninput={(event) => onmetachange('quality', 'min', numeric(event.currentTarget.value))} placeholder="min" /><input type="number" value={metaFilters.quality.max ?? ''} oninput={(event) => onmetachange('quality', 'max', numeric(event.currentTarget.value))} placeholder="max" /></span>
        </div>
      {:else if qualityProperty}
        <span>{qualityProperty.name}: <b>{qualityProperty.value}</b></span>
      {/if}
      {#each propertyFilters ?? [] as prop, index (prop.id)}
        <div class="meta-filter prop-filter" class:off={!prop.enabled}>
          <label title="Aramaya dahil et"><input type="checkbox" checked={prop.enabled} onchange={(event) => onpropertychange(index, 'enabled', event.currentTarget.checked)} /><i></i><span>{prop.name}</span></label>
          <span class="meta-range"><input type="number" value={prop.min ?? ''} oninput={(event) => onpropertychange(index, 'min', numeric(event.currentTarget.value))} placeholder="min" /><input type="number" value={prop.max ?? ''} oninput={(event) => onpropertychange(index, 'max', numeric(event.currentTarget.value))} placeholder="max" /></span>
        </div>
      {/each}
      {#each otherProperties as prop}
        <span>{prop.name}: <b>{prop.value}</b></span>
      {/each}
    </div>
  {/if}
  <div class="mods">
    {#if choices}
      {#each choices as choice (choice.mod.key)}
        <div class="mod-row" class:off={choice.mod.statId && !choice.selected} class:unmatched={!choice.mod.statId} class:fractured={choice.mod.type === 'fractured'} class:crafted={choice.mod.type === 'crafted'} class:desecrated={choice.mod.type === 'desecrated'}>
          <label class="mod-check" title={choice.mod.statId ? choice.mod.statId : 'GGG stat eşleşmesi bulunamadı'}>
            <input type="checkbox" bind:checked={choice.selected} disabled={!choice.mod.statId} onchange={onchange} />
            <i></i>
            <span class="mod-copy">
              <small>{label(choice)}{choice.mod.name ? ` · ${choice.mod.name}` : ''}</small>
              <em>{choice.mod.text}</em>
            </span>
          </label>
          {#if choice.mod.statId}
            <div class="range">
              <input type="number" bind:value={choice.min} placeholder="min" oninput={onchange} />
              <input type="number" bind:value={choice.max} placeholder="max" oninput={onchange} />
            </div>
          {/if}
        </div>
      {/each}
    {:else}
      {#each item.mods ?? [] as mod}
        <div class="plain-mod" class:implicit={mod.type === 'implicit'} class:fractured={mod.type === 'fractured'} class:crafted={mod.type === 'crafted'} class:desecrated={mod.type === 'desecrated'}>
          {#if mod.tier || mod.tiers?.length || mod.name}<small>{['fractured', 'crafted', 'desecrated'].includes(mod.type) ? `${mod.type} ` : ''}{mod.affix || mod.type}{tiers(mod)}{mod.name ? ` · ${mod.name}` : ''}</small>{/if}
          <span>{mod.text}</span>
        </div>
      {/each}
    {/if}
  </div>
  {#if item.unidentified || item.fractured || item.corrupted || item.twiceCorrupted || item.mirrored || item.sanctified}
    <div class="item-states">
      {#if item.unidentified}
        <label class="state-control unidentified"><strong>Unidentified</strong>{#if stateFilters}<select value={stateFilters.unidentified} onchange={(event) => onstatechange('unidentified', event.currentTarget.value)}><option value="">Any</option><option value="true">Yes</option><option value="false">No</option></select>{/if}</label>
      {/if}
      {#if item.fractured}
        <label class="state-control fractured"><strong>Fractured Item</strong>{#if stateFilters}<select value={stateFilters.fractured} onchange={(event) => onstatechange('fractured', event.currentTarget.value)}><option value="">Any</option><option value="true">Yes</option><option value="false">No</option></select>{/if}</label>
      {/if}
      {#if item.corrupted}
        <label class="state-control corrupted"><strong>Corrupted</strong>{#if stateFilters}<select value={stateFilters.corrupted} onchange={(event) => onstatechange('corrupted', event.currentTarget.value)}><option value="">Any</option><option value="true">Yes</option><option value="false">No</option></select>{/if}</label>
      {/if}
      {#if item.twiceCorrupted}
        <label class="state-control corrupted"><strong>Twice Corrupted</strong>{#if stateFilters}<select value={stateFilters.twiceCorrupted} onchange={(event) => onstatechange('twiceCorrupted', event.currentTarget.value)}><option value="">Any</option><option value="true">Yes</option><option value="false">No</option></select>{/if}</label>
      {/if}
      {#if item.mirrored}
        <label class="state-control mirrored"><strong>Mirrored</strong>{#if stateFilters}<select value={stateFilters.mirrored} onchange={(event) => onstatechange('mirrored', event.currentTarget.value)}><option value="">Any</option><option value="true">Yes</option><option value="false">No</option></select>{/if}</label>
      {/if}
      {#if item.sanctified}
        <label class="state-control sanctified"><strong>Sanctified</strong>{#if stateFilters}<select value={stateFilters.sanctified} onchange={(event) => onstatechange('sanctified', event.currentTarget.value)}><option value="">Any</option><option value="true">Yes</option><option value="false">No</option></select>{/if}</label>
      {/if}
    </div>
  {/if}
</article>

<style>
  .item-card { border: 1px solid #4a4030; background: rgba(7,8,9,.88); box-shadow: inset 0 0 32px #000; }
  .item-title { padding: 9px 12px 8px; text-align: center; border-bottom: 1px solid #4a4030; font-family: var(--serif); letter-spacing: .04em; color: #d7b76d; background: linear-gradient(90deg, transparent, rgba(194,151,70,.10), transparent); }
  .item-title strong,.item-title span { display:block; }
  .title-toggle { display:block; width:100%; padding:0; border:0; background:none; color:inherit; font:inherit; letter-spacing:inherit; cursor:pointer; }
  .title-toggle:hover { filter:brightness(1.25); }
  .title-toggle.off { opacity:.35; text-decoration:line-through; }
  .rarity-select { display:flex; align-items:center; gap:5px; color:#c8c1aa; }
  .rarity-select.off span { opacity:.45; }
  .rarity-select select { padding:3px 16px 3px 5px; border:1px solid #4b473b; border-radius:2px; background:#191b18; color:var(--gold-bright); font-family:var(--sans); font-size:10px; text-transform:none; }
  .prop-filter>label{color:#8192b4;text-transform:uppercase}
  .mod-row.off .mod-copy { opacity:.5; }
  .item-title strong { font-size: 17px; }
  .item-title.unique { color:#d68d45; border-color:#7a4d22; }
  .item-title.rare { color:#d7d08a; }
  .item-meta { display:flex; justify-content:center; align-items:center; gap:10px; flex-wrap:wrap; padding:8px 10px 5px; color:var(--muted); font-size:11px; text-transform:uppercase; }
  .item-meta b { color:var(--text); }
  .item-class{padding:0 3px}.properties { display:flex; justify-content:center; align-items:center; gap:10px; flex-wrap:wrap; padding:3px 10px 8px; color:#8192b4; font-size:11px; border-bottom:1px solid #29251e; }
  .meta-filter{display:flex;align-items:center;gap:5px;color:#c8c1aa}.meta-filter.off{opacity:.45}.meta-filter>label{display:flex;align-items:center;gap:5px;cursor:pointer;white-space:nowrap}.meta-filter>label input{position:absolute;opacity:0}.meta-filter>label i{flex:0 0 9px;width:9px;height:9px;transform:rotate(45deg);border:1px solid #766b4f;background:#090a0b}.meta-filter>label input:checked+i{background:var(--gold);box-shadow:inset 0 0 0 2px #151615}.meta-range{display:grid;grid-template-columns:44px 44px;gap:3px}.meta-range input{box-sizing:border-box;min-width:0;width:100%;padding:4px 3px;border:1px solid #3d3a31;border-radius:2px;background:#111313;color:var(--gold-bright);font-size:10px}.quality-filter>label{color:#8192b4;text-transform:uppercase}
  .mods { padding:7px 8px 9px; }
  .mod-row { display:grid; grid-template-columns:minmax(0,1fr) 112px; gap:7px; align-items:center; padding:5px 0; border-bottom:1px solid rgba(255,255,255,.035); }
  .mod-row:last-child { border-bottom:0; }
  .mod-check { display:flex; gap:7px; align-items:flex-start; min-width:0; cursor:pointer; }
  .mod-check input { position:absolute; opacity:0; pointer-events:none; }
  .mod-check i { flex:0 0 13px; width:13px; height:13px; margin-top:9px; transform:rotate(45deg); border:1px solid var(--gold-dim); background:#090a0b; }
  .mod-check input:checked + i { background:var(--gold); box-shadow:inset 0 0 0 3px #17191b; }
  .mod-copy { min-width:0; }
  .mod-copy small,.plain-mod small { display:block; color:#9f77b7; font-size:10px; text-transform:uppercase; }
  .mod-row.fractured .mod-copy small,.plain-mod.fractured small { color:#9ed0d8; }
  .mod-row.crafted .mod-copy small,.plain-mod.crafted small { color:#b892c8; }
  .mod-row.desecrated .mod-copy small,.plain-mod.desecrated small { color:#d68869; }
  .mod-copy em { display:block; color:#9aa8d2; font-style:normal; white-space:pre-line; line-height:1.25; }
  .range { display:grid; grid-template-columns:1fr 1fr; gap:4px; }
  .range input { min-width:0; width:100%; padding:5px 4px; background:#111313; border:1px solid #3d3a31; color:var(--gold-bright); border-radius:2px; }
  .unmatched { opacity:.48; }
  .plain-mod { padding:4px 5px; text-align:center; color:#9aa8d2; white-space:pre-line; }
  .plain-mod.implicit { color:#7388c2; }
  .plain-mod.crafted { color:#9a78b2; }
  .compact .mods { max-height:300px; overflow:auto; }
  .item-states { display:flex; justify-content:center; align-items:center; flex-wrap:wrap; gap:8px 14px; padding:7px 10px; border-top:1px solid #3b3025; font-family:var(--serif); font-size:11px; text-transform:uppercase; letter-spacing:.08em; }
  .state-control{display:flex;align-items:center;gap:7px}.state-control select{width:72px;padding:4px 18px 4px 6px;border:1px solid #4b473b;border-radius:2px;background:#191b18;color:#c6c2ad;font-family:var(--sans);font-size:10px;text-transform:none;letter-spacing:0}
  .item-states .unidentified { color:#d54a45; }
  .item-states .corrupted { color:#d54a45; }
  .item-states .fractured { color:#9ed0d8; }
  .item-states .sanctified { color:#d7bd74; }
  .item-states .mirrored { color:#a9b8e8; }
</style>
