<script lang="ts">
  import type { ExchangeResult } from '../../bindings/poe2filter/internal/trade/models'
  import { currencyInfo } from './currencies.svelte'
  import { currencyLabel, listedAgo } from './overlayQuery'
  import { t } from './i18n.svelte'

  // Bulk exchange offers (Ange's market) for items the item search cannot
  // find: each row is a seller's ratio, shown as the price of one item.
  let { result, loading, error }: { result: ExchangeResult | null; loading: boolean; error: string } = $props()

  function amount(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return '0'
    if (n >= 100) return n.toFixed(0)
    if (n >= 10) return n.toFixed(1).replace(/\.0$/, '')
    if (n >= 1) return n.toFixed(2).replace(/\.?0+$/, '')
    return n.toPrecision(2)
  }
</script>

<div class="results-head">
  <span>{loading ? t('ov.searching') : t('ov.ex.offers', result?.total ?? 0)}</span>
  {#if result?.tradeUrl}<button type="button" onclick={() => window.dispatchEvent(new CustomEvent('open-trade', { detail: result!.tradeUrl }))}>pathofexile.com/trade ↗</button>{/if}
</div>
{#if error}<p class="result-error">{error}</p>{/if}
{#if loading}<div class="loading"><i></i><span></span><i></i></div>{/if}
{#if !loading && result?.offers?.length}
  <div class="table">
    <div class="row head"><span>{t('ov.ex.each')}</span><span>{t('ov.ex.ratio')}</span><span>{t('ov.cc.stock')}</span><span>{t('ov.col.account')}</span><span>{t('ov.col.listed')}</span></div>
    {#each result.offers as offer, i (i)}
      {@const coin = currencyInfo(offer.currency)}
      <div class="row">
        <strong class="price">{amount(offer.pay / offer.get)}{#if coin?.image}<i>×</i><img src={coin.image} alt={coin.text} />{:else} <small>{currencyLabel(offer.currency)}</small>{/if}</strong>
        <span title={t('ov.ex.ratioTitle')}>{offer.pay}:{offer.get}</span>
        <span>{offer.stock || ''}</span>
        <span class="account">{offer.account}</span>
        <span>{listedAgo(offer.indexed)}</span>
      </div>
    {/each}
  </div>
{:else if !loading && result}
  <p class="empty">{t('ov.ex.none')}</p>
{/if}

<style>
  .results-head { display:flex; align-items:center; justify-content:space-between; min-height:31px; color:var(--muted); font-size:11px; }
  .results-head button { border:0; background:none; color:var(--gold); font-size:11px; }
  .results-head button:hover { color:var(--gold-bright); }
  .result-error { margin:6px 0; color:#e08a7a; }
  .empty { margin:8px 0; padding:22px 10px; border:1px solid #2c2d28; color:var(--muted); text-align:center; }
  .table { border:1px solid #34342e; }
  .row { display:grid; grid-template-columns:1.2fr .8fr .6fr 1.5fr .8fr; align-items:center; gap:6px; padding:5px 8px; border-top:1px solid #23241f; color:#b8b4a2; font-size:11px; }
  .row:first-child { border-top:0; }
  .row.head { color:#7b7f76; font-size:9px; text-transform:uppercase; letter-spacing:.06em; background:#121411; }
  .price { display:flex; align-items:center; gap:3px; color:var(--gold-bright); font-weight:normal; }
  .price i { color:var(--muted); font-style:normal; font-size:9px; }
  .price img { width:20px; height:20px; object-fit:contain; }
  .account { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .loading { display:flex; justify-content:center; gap:6px; padding:14px; }
  .loading i,.loading span { width:7px; height:7px; transform:rotate(45deg); background:var(--gold-dim); }
</style>
