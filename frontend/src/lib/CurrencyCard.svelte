<script lang="ts">
  import type { CurrencyQuote, Item } from '../../bindings/poe2filter/internal/overlay/models'
  import { listedAgo } from './overlayQuery'

  let { item, quote }: { item: Item; quote: CurrencyQuote } = $props()

  const stock = $derived(Math.max(1, item.stackSize || 1))
  const totalEx = $derived(quote.valueEx * stock)
  const totalDiv = $derived(quote.divineEx > 0 ? totalEx / quote.divineEx : 0)
  // The divine itself reads better in exalted; everything else in both.
  const isDivine = $derived(quote.name.toLowerCase() === 'divine orb')
  // "1 div = N of these" for cheap currency, "1 of these = N div" for dear.
  const perDivine = $derived(quote.valueEx > 0 && quote.divineEx > 0 ? quote.divineEx / quote.valueEx : 0)

  const updated = $derived.by(() => {
    const ago = listedAgo(quote.generatedAt)
    return ago === '0m' ? 'az önce' : `${ago} önce`
  })

  function amount(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return '0'
    if (n >= 1000) return Math.round(n).toLocaleString('en-US')
    if (n >= 100) return n.toFixed(0)
    if (n >= 10) return n.toFixed(1).replace(/\.0$/, '')
    if (n >= 1) return n.toFixed(2).replace(/\.?0+$/, '')
    return n.toPrecision(2)
  }
</script>

<section class="currency-card">
  <div class="title">
    <strong>{quote.name}</strong>
    <span class="stock">Stock <b>{stock.toLocaleString('en-US')}</b></span>
  </div>
  <div class="worth">
    <div class="side">
      <b>{stock.toLocaleString('en-US')}</b>
      <small>{quote.name}</small>
    </div>
    <span class="swap">⇄</span>
    <div class="side total">
      {#if totalDiv >= 0.1 && !isDivine}
        <b>≈ {amount(totalDiv)} <i>div</i></b>
        <small>{amount(totalEx)} ex</small>
      {:else}
        <b>≈ {amount(totalEx)} <i>ex</i></b>
        {#if isDivine || totalDiv > 0}<small>{amount(totalDiv)} div</small>{/if}
      {/if}
    </div>
  </div>
  <div class="rates">
    <span>Unit <b>{amount(quote.valueEx)}</b> ex</span>
    {#if perDivine >= 1 && !isDivine}
      <span><b>{amount(perDivine)}</b> per div</span>
    {:else if perDivine > 0 && !isDivine}
      <span><b>{amount(1 / perDivine)}</b> div each</span>
    {/if}
  </div>
  <p class="source" title="Filtrenin kullandığı fiyat listesi">Fiyat listesi · {quote.league} · {updated}</p>
</section>

<style>
  .currency-card { border: 1px solid #4a4030; background: rgba(7,8,9,.88); box-shadow: inset 0 0 32px #000; }
  .title { padding: 9px 12px 8px; text-align: center; border-bottom: 1px solid #4a4030; background: linear-gradient(90deg, transparent, rgba(194,151,70,.10), transparent); }
  .title strong { display: block; font-family: var(--serif); letter-spacing: .04em; color: #d7b76d; font-size: 14px; }
  .stock { display: inline-block; margin-top: 5px; padding: 1px 7px; border: 1px solid #3e3a2f; background: #1b1c19; color: var(--muted); font-size: 10px; text-transform: uppercase; }
  .stock b { color: var(--gold-bright); }
  .worth { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 8px; padding: 12px 12px 8px; }
  .side { display: grid; justify-items: center; gap: 2px; text-align: center; }
  .side b { color: var(--gold-bright); font-size: 17px; }
  .side b i { color: var(--gold); font-size: 11px; font-style: normal; }
  .side small { color: var(--muted); font-size: 10px; }
  .total b { color: #f0e0b0; }
  .swap { color: var(--gold-dim); font-size: 18px; }
  .rates { display: flex; justify-content: center; gap: 14px; padding: 0 12px 8px; color: var(--muted); font-size: 10px; }
  .rates b { color: var(--gold-bright); }
  .source { margin: 0; padding: 5px 12px 7px; border-top: 1px solid #2a2820; color: #686e74; font-size: 9px; text-align: center; }
</style>
