<script lang="ts">
  import type { FilterExplanation } from '../../bindings/poe2filter/internal/app/models'
  import type { Block, Match } from '../../bindings/poe2filter/internal/filtereval/models'
  import { listedAgo } from './overlayQuery'
  import { t } from './i18n.svelte'

  // What the written loot filter does with the item: one line, details on
  // click. worthEx is the item's price as Alt+E found it (0 = unknown), to
  // warn about a hidden item worth more than the filter's threshold.
  let { explain, worthEx = 0 }: { explain: FilterExplanation; worthEx?: number } = $props()

  let open = $state(false)

  const verdict = $derived(explain.verdict as string)
  const writtenAgo = $derived.by(() => {
    const when = Date.parse(explain.writtenAt)
    if (Number.isFinite(when) && Date.now() - when < 60000) return t('ov.cc.justNow')
    return t('ov.cc.ago', listedAgo(explain.writtenAt))
  })
  const hiddenButWorth = $derived(verdict === 'hide' && explain.thresholdEx > 0 && worthEx >= explain.thresholdEx)

  function ruleName(b: Block): string {
    const section = b.section || t('fx.unnamed')
    if (b.source === 'ours') return section
    const tags = [b.type, b.tier].filter(Boolean).join(' / ')
    return `NeverSink · ${section}${tags ? ` (${tags})` : ''}`
  }

  // Conditions the copied text cannot settle, in words.
  function unknownLabel(line: string): string {
    const key = line.trim().split(/\s+/)[0]
    switch (key) {
      case 'UnidentifiedItemTier': return t('fx.unk.tier')
      case 'AreaLevel': return t('fx.unk.area')
      case 'Width': case 'Height': return t('fx.unk.size')
      case 'BaseArmour': case 'BaseEvasion': case 'BaseEnergyShield': return t('fx.unk.base')
      case 'DropLevel': return t('fx.unk.drop')
    }
    return line.trim()
  }

  function unknownList(m: Match): string {
    return [...new Set((m.unknown ?? []).map(unknownLabel))].join(', ')
  }

  function money(ex: number): string {
    if (explain.divineEx > 0 && ex >= explain.divineEx * 0.5) return `${(ex / explain.divineEx).toFixed(1).replace(/\.0$/, '')} div`
    return `${ex >= 10 ? Math.round(ex) : ex.toFixed(1).replace(/\.0$/, '')} ex`
  }
</script>

<section class="verdict" class:hide={verdict === 'hide'} class:show={verdict === 'show'} class:minimal={verdict === 'minimal'}>
  <button type="button" class="line" onclick={() => (open = !open)} title={t('fx.details')}>
    <b>{t(`fx.${verdict}`)}</b>
    {#if explain.final}<span class="rule">{ruleName(explain.final.block)}</span>{/if}
    {#if explain.maybe?.length}<i class="maybe" title={t('fx.maybeTitle')}>?</i>{/if}
    <em>{open ? '▴' : '▾'}</em>
  </button>
  {#if hiddenButWorth}
    <p class="warn">⚠ {t('fx.worthHidden', money(worthEx), money(explain.thresholdEx))}</p>
  {/if}
  {#if open}
    <div class="details">
      {#if explain.maybe?.length}
        {#each explain.maybe as m (m.block.line)}
          <p class="maybe-row">{t('fx.maybeRow', t(`fx.action.${m.block.action}`), ruleName(m.block), unknownList(m))}</p>
        {/each}
      {/if}
      {#if explain.final}
        <p class="label">{t('fx.rule')} · {t('fx.line', explain.final.block.line)}</p>
        <pre>{explain.final.block.text}</pre>
      {/if}
      {#each explain.decorations ?? [] as d (d.block.line)}
        <p class="decor">+ {ruleName(d.block)}</p>
      {/each}
      <p class="meta">
        {#if explain.areaLevel > 0}{t('fx.area', explain.area, explain.areaLevel)}{:else}{t('fx.areaUnknown')}{/if}
      </p>
      <p class="meta">{t('fx.written', writtenAgo)}</p>
    </div>
  {/if}
</section>

<style>
  .verdict { margin-top: 5px; border: 1px solid var(--ui-line,#34342e); background: var(--ui-sunk,#111311); }
  .line { display: flex; align-items: center; gap: 8px; width: 100%; padding: 4px 10px; border: 0; background: none; color: var(--muted); text-align: left; font-size: 11px; }
  .line:hover { background: var(--ui-hover,#181a17); }
  .line b { flex: 0 0 auto; font-weight: 600; }
  .show .line b { color: var(--ui-ok,#8fd18a); }
  .hide .line b { color: var(--ui-bad,#c98a7a); }
  .minimal .line b { color: var(--ui-gold-bright,#c9b98a); }
  .rule { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ui-gold-bright,#b8b4a2); }
  .maybe { flex: 0 0 auto; width: 15px; height: 15px; border: 1px solid var(--ui-line-strong,#766b4f); border-radius: 50%; color: var(--gold); font-style: normal; font-size: 10px; line-height: 13px; text-align: center; }
  .line em { flex: 0 0 auto; font-style: normal; color: var(--ui-muted,#6f746c); }
  .warn { margin: 0; padding: 6px 10px; border-top: 1px solid var(--ui-line-strong,#5a3a2e); background: var(--ui-surface-2,#2a1a14); color: var(--ui-warn,#f0b09a); font-size: 11px; }
  .details { padding: 4px 10px 8px; border-top: 1px solid var(--ui-line,#2a2820); font-size: 10px; color: var(--muted); }
  .details p { margin: 4px 0; }
  .maybe-row { color: var(--ui-gold-bright,#c9b98a); }
  .label { color: var(--ui-muted,#8a8d84); text-transform: uppercase; letter-spacing: .06em; font-size: 9px; }
  pre { margin: 2px 0 6px; padding: 6px 8px; max-height: 160px; overflow: auto; background: var(--ui-sunk,#0a0b0a); border: 1px solid var(--ui-line,#23241f); color: var(--ui-gold-bright,#b8b4a2); font-size: 10px; white-space: pre-wrap; }
  .decor { color: var(--ui-text-2,#9aa0a6); }
  .meta { color: var(--ui-muted,#6f746c); }
</style>
