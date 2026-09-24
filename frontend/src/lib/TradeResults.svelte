<script lang="ts">
  import type { Evaluation, EvaluatedListing } from '../../bindings/poe2filter/internal/trade/models'
  import { currencyLabel, listedAgo } from './overlayQuery'

  let { result = null, loading = false, error = '', expanded = false }: { result?: Evaluation | null; loading?: boolean; error?: string; expanded?: boolean } = $props()
  let preview = $state<EvaluatedListing | null>(null)
  // Weapons get a DPS column, as on the trade site.
  const hasDps = $derived(result?.listings?.some((row) => row.item.dps > 0) ?? false)

  function toggle(row: EvaluatedListing) {
    preview = preview?.id === row.id ? null : row
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
      {#if row.item.itemLevel}<span>Item Level <b>{row.item.itemLevel}</b></span>{/if}
    </div>
    <div class="preview-props">
      {#each row.item.properties ?? [] as prop}<span>{prop.name}{#if prop.value}: <b>{prop.value}</b>{/if}</span>{/each}
    </div>
    <div class="preview-mods">
      {#each row.item.mods ?? [] as mod}
        <p class:type-implicit={mod.type === 'implicit'} class:type-fractured={mod.type === 'fractured'} class:type-crafted={mod.type === 'crafted'} class:type-desecrated={mod.type === 'desecrated'} class:type-rune={mod.type === 'rune'}>
          {#if mod.tier || mod.name}<small>{mod.tier} {mod.name}</small>{/if}
          {mod.description}
        </p>
      {/each}
    </div>
    {#if row.item.unidentified || row.item.fractured || row.item.corrupted || row.item.sanctified}
      <div class="item-states">
        {#if row.item.unidentified}<strong class="unidentified">Unidentified</strong>{/if}
        {#if row.item.fractured}<strong class="fractured">Fractured Item</strong>{/if}
        {#if row.item.corrupted}<strong class="corrupted">Corrupted</strong>{/if}
        {#if row.item.sanctified}<strong class="sanctified">Sanctified</strong>{/if}
      </div>
    {/if}
  </div>
{/snippet}

<div class="results-head">
  <span>{loading ? 'Searching…' : `${result?.total ?? 0} results`}</span>
  {#if result?.tradeUrl}<button type="button" onclick={() => window.dispatchEvent(new CustomEvent('open-trade', { detail: result!.tradeUrl }))}>pathofexile.com/trade ↗</button>{/if}
</div>
{#if error}<p class="result-error">{error}</p>{/if}
{#if loading}<div class="loading"><i></i><span></span><i></i></div>{/if}
{#if !loading && result?.listings?.length}
  <div class="result-table" class:expanded class:dps={hasDps}>
    {#if !expanded}<div class="table-head"><span></span><span>Price</span><span>iLvl</span>{#if hasDps}<span>DPS</span>{/if}<span>Account</span><span>Listed</span></div>{/if}
    {#each result.listings as row (row.id)}
      <article class="listing-card" class:full={expanded}>
        {#if expanded}{@render itemPreview(row)}{/if}
        <div class="listing" class:on={!expanded && preview?.id === row.id}>
          <button type="button" class="eye" title={expanded ? 'İlan' : 'Itemı göster'} onclick={(event) => { event.stopPropagation(); if (!expanded) toggle(row) }}>{expanded ? '●' : '◉'}</button>
          <strong>{row.amount} <small>{currencyLabel(row.currency)}</small></strong>
          <span>{row.item.itemLevel}</span>
          {#if hasDps}<span class="dps" title={row.item.dps ? `pDPS ${row.item.physicalDps} · eDPS ${row.item.elementalDps}` : ''}>{row.item.dps ? Math.round(row.item.dps) : ''}</span>{/if}
          <span class="account">{row.account}</span>
          <span>{listedAgo(row.listed)}</span>
          <button type="button" class="hideout" disabled title={row.hideoutToken ? 'Iteme özel seyahat için güvenli Path of Exile oturum bağlantısı gerekiyor' : 'Bu yanıtta hideout token yok'}>↪</button>
        </div>
        {#if !expanded && preview?.id === row.id}{@render itemPreview(row)}{/if}
      </article>
    {/each}
  </div>
{:else if !loading && result}
  <p class="empty">Bu filtrelerle ilan bulunamadı.</p>
{/if}

<style>
  .results-head { display:flex; align-items:center; justify-content:space-between; min-height:31px; color:var(--muted); font-size:11px; }
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
  .eye,.hideout { min-width:0;height:22px;padding:0;color:#a99c72;border:1px solid #56523f;text-align:center;border-radius:2px;background:#181a17; }
  .hideout { color:#d6ccb0;font-size:15px;background:#696855; }
  .hideout:hover { background:#89856d;color:#fff; }.hideout:disabled{opacity:.3}
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
  .preview p small { display:block; color:#9d76b6; text-transform:uppercase; }
  .preview p.type-implicit { color:#7188c4; }
  .preview p.type-fractured { color:#9ed0d8; }
  .preview p.type-crafted { color:#9d76b6; }
  .preview p.type-desecrated { color:#d68869; }
  .preview p.type-rune { color:#7e899d; }
  .item-states { display:flex;justify-content:center;gap:12px;margin-top:8px;padding-top:8px;border-top:1px solid #3b3025;font-family:var(--serif);font-size:10px;text-transform:uppercase;letter-spacing:.08em; }
  .item-states .unidentified { color:#d54a45; }.item-states .fractured { color:#9ed0d8; }.item-states .corrupted { color:#d54a45; }.item-states .sanctified { color:#d7bd74; }
  .loading { display:flex; justify-content:center; gap:5px; padding:18px; }
  .loading i,.loading span { width:7px; height:7px; transform:rotate(45deg); background:var(--gold-dim); animation:pulse 1s infinite alternate; }
  .loading span { animation-delay:.2s; }.loading i:last-child{animation-delay:.4s}
  .empty { text-align:center; color:var(--muted); padding:16px; border:1px solid #2b2d2b; }
  @keyframes pulse { to { background:var(--gold-bright); } }
</style>
