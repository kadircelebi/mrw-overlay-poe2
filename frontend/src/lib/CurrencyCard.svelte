<script lang="ts">
  import type { CurrencyQuote, Item } from '../../bindings/poe2filter/internal/overlay/models'
  import { listedAgo } from './overlayQuery'
  import { t } from './i18n.svelte'

  let { item, quote }: { item: Item; quote: CurrencyQuote } = $props()

  const stock = $derived(Math.max(1, item.stackSize || 1))
  // The amount priced: the stack by default, any other count when typed in.
  let qty = $derived(stock)
  const totalEx = $derived(quote.valueEx * qty)
  const totalDiv = $derived(quote.divineEx > 0 ? totalEx / quote.divineEx : 0)
  // The divine itself reads better in exalted; everything else in both.
  const isDivine = $derived(quote.name.toLowerCase() === 'divine orb')
  // "1 div = N of these" for cheap currency, "1 of these = N div" for dear.
  const perDivine = $derived(quote.valueEx > 0 && quote.divineEx > 0 ? quote.divineEx / quote.valueEx : 0)

  const updated = $derived.by(() => {
    const when = Date.parse(quote.generatedAt)
    if (Number.isFinite(when) && Date.now() - when < 60000) return t('ov.cc.justNow')
    return t('ov.cc.ago', listedAgo(quote.generatedAt))
  })

  function typeQty(event: Event) {
    const n = Math.floor(Number((event.currentTarget as HTMLInputElement).value))
    if (Number.isFinite(n) && n >= 1) qty = Math.min(n, 999999)
  }

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
  <div class="title" data-rarity={item.rarity?.toLowerCase()}>
    <strong>{quote.name}</strong>
    <button type="button" class="stock" class:changed={qty !== stock} disabled={qty === stock}
      onclick={() => (qty = stock)} title={qty !== stock ? t('ov.cc.qtyReset') : ''}>{t('ov.cc.stock')} <b>{stock.toLocaleString('en-US')}</b></button>
  </div>
  <div class="worth">
    <div class="side">
      <input class="qty" type="number" min="1" max="999999" step="1" value={qty} oninput={typeQty}
        onblur={(e) => (e.currentTarget.value = String(qty))} onfocus={(e) => e.currentTarget.select()}
        title={t('ov.cc.qtyTitle')} aria-label={t('ov.cc.qtyTitle')} />
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
    <span>{t('ov.cc.unit')} <b>{amount(quote.valueEx)}</b> ex</span>
    {#if perDivine >= 1 && !isDivine}
      <span><b>{amount(perDivine)}</b> {t('ov.cc.perDiv')}</span>
    {:else if perDivine > 0 && !isDivine}
      <span><b>{amount(1 / perDivine)}</b> {t('ov.cc.divEach')}</span>
    {/if}
  </div>
  <p class="source" title={t('ov.cc.sourceTitle')}>{t('ov.cc.source', quote.league, updated)}</p>
</section>

<style>
  .currency-card { border: 1px solid var(--ui-item-line,#4a4030); background: var(--ui-item-bg,rgba(7,8,9,.88)); box-shadow: inset 0 0 32px var(--ui-shadow,#000); }
  .title { padding: 9px 12px 8px; text-align: center; border-bottom: 1px solid var(--ui-line-strong,#4a4030); background: linear-gradient(90deg, transparent, var(--ui-tint,rgba(194,151,70,.10)), transparent); }
  .title strong { display: block; font-family: var(--serif); letter-spacing: .04em; color:#c8c8c8; font-size: 14px; }
  .title[data-rarity='rare'] strong{color:#ebe27a}.title[data-rarity='magic'] strong{color:#8f94ff}.title[data-rarity='unique'] strong{color:#d68d45}
  .stock { display: inline-block; margin-top: 5px; padding: 1px 7px; border: 1px solid var(--ui-line,#3e3a2f); background: var(--ui-surface,#1b1c19); color: var(--muted); font: inherit; font-size: 10px; text-transform: uppercase; }
  .stock b { color: var(--gold-bright); }
  .worth { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 8px; padding: 12px 12px 8px; }
  .side { display: grid; justify-items: center; gap: 2px; text-align: center; }
  .side b { color: var(--gold-bright); font-size: 17px; }
  .side b i { color: var(--gold); font-size: 11px; font-style: normal; }
  .side small { color: var(--muted); font-size: 10px; }
  .qty { width: 92px; padding: 1px 4px; border: 1px solid var(--ui-line,#3e3a2f); background: var(--ui-surface,#1b1c19); color: var(--gold-bright); font: inherit; font-size: 17px; font-weight: 700; text-align: center; -moz-appearance: textfield; appearance: textfield; }
  .qty::-webkit-inner-spin-button, .qty::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
  .qty:hover, .qty:focus { border-color: var(--gold-dim); outline: none; }
  .stock:disabled { cursor: default; }
  .stock.changed { border-color: var(--gold-dim); cursor: pointer; }
  .stock.changed:hover { color: var(--gold-bright); }
  .total b { color: var(--ui-gold-bright,#f0e0b0); }
  .swap { color: var(--gold-dim); font-size: 18px; }
  .rates { display: flex; justify-content: center; gap: 14px; padding: 0 12px 8px; color: var(--muted); font-size: 10px; }
  .rates b { color: var(--gold-bright); }
  .source { margin: 0; padding: 5px 12px 7px; border-top: 1px solid var(--ui-line,#2a2820); color: var(--ui-muted,#686e74); font-size: 9px; text-align: center; }
</style>
