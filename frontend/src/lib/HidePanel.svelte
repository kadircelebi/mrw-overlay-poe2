<script lang="ts">
  import { AppService } from '../../bindings/poe2filter'
  import type { FilterExplanation } from '../../bindings/poe2filter/models'
  import type { Catalog, CurrencyQuote, Item } from '../../bindings/poe2filter/internal/overlay/models'
  import type { HiddenItem } from '../../bindings/poe2filter/internal/filter/models'
  import FilterVerdict from './FilterVerdict.svelte'
  import { t } from './i18n.svelte'

  // Alt+H: what to hide of the item under the cursor. The choices depend on
  // the kind of item; the narrowest one is preselected.
  let { item, quote, explain, catalog }: { item: Item; quote: CurrencyQuote | null; explain: FilterExplanation | null; catalog: Catalog | null } = $props()

  type Scope = 'rarity' | 'gear' | 'all' | 'below'
  const rarity = $derived(item.rarity?.toLowerCase() ?? '')
  const isStack = $derived(item.stackSize > 0 || !!item.exchange)
  const isUnique = $derived(rarity === 'unique')
  const isGear = $derived(['normal', 'magic', 'rare'].includes(rarity) && !isStack)
  const base = $derived(item.baseType || item.name)

  let scope = $state<Scope>('all')
  let below = $state(10)
  let whileCheap = $state(true)
  let busy = $state(false)
  let error = $state('')
  let done = $state<HiddenItem | null>(null)
  let updating = $state(true)

  // Start from the narrowest choice whenever a new item arrives.
  $effect(() => {
    scope = isGear ? 'rarity' : 'all'
    below = Math.max(2, (item.stackSize || 0) + 1, 10)
    whileCheap = true
    done = null
    error = ''
  })

  const worthEx = $derived(quote ? quote.valueEx * Math.max(1, item.stackSize || 1) : 0)
  const valuable = $derived(!!explain && explain.thresholdEx > 0 && quote !== null && quote.valueEx >= explain.thresholdEx)
  // NeverSink's exotic rule wins over hiding (the player keeps it by leaving
  // that rule on), so hiding such an item would do nothing.
  const exotic = $derived(!!explain?.final && explain.final.block.source === 'ours' && ['exoticbases', 'exoticmods'].includes(explain.final.block.type ?? ''))

  // Uniques the filter cannot tell apart from this one: they share its base.
  const siblings = $derived.by(() => {
    if (!isUnique || !catalog) return []
    const names = new Set<string>()
    for (const group of catalog.items ?? []) for (const e of group.entries ?? []) {
      if (e.name && e.type === base && e.name !== item.name) names.add(e.name)
    }
    return [...names].slice(0, 6)
  })

  function entry(): HiddenItem {
    const rarities = isUnique ? ['Unique'] : !isGear ? [] : scope === 'rarity' ? [cap(rarity)] : ['Normal', 'Magic', 'Rare']
    return { base, rarities, below_stack: isStack && scope === 'below' ? below : 0, while_cheap: !isStack || whileCheap, added_at: 0 }
  }

  function cap(s: string): string {
    return s.charAt(0).toUpperCase() + s.slice(1)
  }

  async function hide() {
    if (busy) return
    if (valuable && !whileCheap && !confirm(t('hide.confirmValuable', base))) return
    busy = true
    error = ''
    try {
      const e = entry()
      const res = await AppService.HideItem(e)
      done = e
      updating = res.updating
    } catch (err) {
      error = String(err).replace(/^RuntimeError:\s*/i, '')
    } finally {
      busy = false
    }
  }

  async function undo() {
    if (!done || busy) return
    busy = true
    try {
      await AppService.UnhideItem(done)
      done = null
    } catch (err) {
      error = String(err).replace(/^RuntimeError:\s*/i, '')
    } finally {
      busy = false
    }
  }

  function money(ex: number): string {
    const div = explain?.divineEx ?? 0
    if (div > 0 && ex >= div * 0.5) return `${(ex / div).toFixed(1).replace(/\.0$/, '')} div`
    return `${ex >= 10 ? Math.round(ex) : ex.toFixed(1).replace(/\.0$/, '')} ex`
  }
</script>

<section class="hide-panel">
  <div class="title">
    <small>{t('hide.title')}</small>
    <strong>{item.name && item.name !== base ? `${item.name} · ${base}` : base}</strong>
    <span>{item.rarity}{#if item.stackSize} · {t('ov.cc.stock')} {item.stackSize}{/if}</span>
  </div>

  {#if explain}<FilterVerdict {explain} {worthEx} />{/if}

  {#if done}
    <p class="done">✓ {t(updating ? 'hide.done' : 'hide.doneLater', base)}</p>
    <div class="actions"><button type="button" disabled={busy} onclick={undo}>{t('hide.undo')}</button></div>
  {:else if exotic}
    <p class="note">{t('hide.exotic')}</p>
  {:else}
    <div class="choices">
      {#if isGear}
        <label><input type="radio" bind:group={scope} value="rarity" /> {t('hide.onlyRarity', cap(rarity))}</label>
        <label><input type="radio" bind:group={scope} value="gear" /> {t('hide.gearAll')}</label>
      {:else if isStack}
        <label><input type="radio" bind:group={scope} value="all" /> {t('hide.allStacks')}</label>
        <label class="below"><input type="radio" bind:group={scope} value="below" /> {t('hide.below')}
          <input type="number" min="2" max="5000" bind:value={below} onfocus={() => (scope = 'below')} /></label>
        <label class="check"><input type="checkbox" bind:checked={whileCheap} /> {t('hide.whileCheap')}</label>
        <p class="hint">{t('hide.whileCheapHint')}</p>
      {:else if isUnique}
        <p class="hint">{t('hide.unique', base)}</p>
        {#if siblings.length}<p class="warn">{t('hide.siblings', siblings.join(', '))}</p>{/if}
      {:else}
        <p class="hint">{t('hide.base', base)}</p>
      {/if}
      {#if valuable}<p class="warn">{t(whileCheap ? 'hide.valuableCheap' : 'hide.valuable', money(worthEx))}</p>{/if}
    </div>
    <div class="actions">
      <button type="button" class="primary" disabled={busy} onclick={hide}>{busy ? '…' : t('hide.do')}</button>
      <button type="button" onclick={() => AppService.HideOverlay()}>{t('hide.cancel')}</button>
    </div>
  {/if}
  {#if error}<p class="warn">{error}</p>{/if}
  <p class="foot">{t('hide.reload')}</p>
</section>

<style>
  .hide-panel { border: 1px solid #4a4030; background: rgba(7,8,9,.88); padding-bottom: 6px; }
  .title { display: grid; gap: 3px; padding: 10px 12px 8px; text-align: center; border-bottom: 1px solid #4a4030; background: linear-gradient(90deg, transparent, rgba(194,151,70,.10), transparent); }
  .title small { color: #c98a7a; font-size: 10px; letter-spacing: .04em; }
  .title strong { font-family: var(--serif); color: #d7b76d; font-size: 14px; letter-spacing: .03em; }
  .title span { color: var(--muted); font-size: 10px; text-transform: capitalize; }
  .hide-panel :global(.verdict) { margin: 8px 10px 0; }
  .choices { display: grid; gap: 6px; padding: 10px 12px 4px; color: #c6c2ad; font-size: 12px; }
  .choices label { display: flex; align-items: center; gap: 8px; }
  .choices .below input[type=number] { width: 70px; padding: 3px 6px; border: 1px solid #4b473b; background: #191b18; color: #c6c2ad; }
  .check { margin-top: 4px; }
  .hint { margin: 0 0 0 22px; color: var(--muted); font-size: 10px; }
  .choices > .hint:first-child { margin-left: 0; }
  .warn { margin: 6px 12px 0; padding: 6px 8px; border: 1px solid #5a3a2e; background: #2a1a14; color: #f0b09a; font-size: 11px; }
  .choices .warn { margin: 4px 0 0; }
  .note, .done { margin: 10px 12px 0; color: #c6c2ad; font-size: 12px; }
  .done { color: #8fd18a; }
  .actions { display: flex; gap: 8px; padding: 10px 12px 2px; }
  .actions button { flex: 1; padding: 8px; border: 1px solid #56523f; background: #171917; color: #b8ae91; font-family: var(--serif); }
  .actions button.primary { border-color: #8c5a4a; color: #f0c0b0; }
  .actions button:hover:not(:disabled) { background: #25261f; }
  .foot { margin: 8px 12px 0; color: #686e74; font-size: 9px; text-align: center; }
</style>
